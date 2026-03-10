// Package lan provides LAN sharing with mDNS announcement.
// When enabled, broadcasts service availability via multicast DNS (RFC 6762)
// so teammates on the same network can discover and access local services.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/lan
package lan

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	mdns "github.com/miekg/dns"
)

// LANInfo holds LAN sharing configuration.
type LANInfo struct {
	LocalIP  string `json:"local_ip"`
	LocalURL string `json:"local_url"`
	LANURL   string `json:"lan_url"`
}

// Responder implements a simple mDNS responder that answers queries
// for *.local names, mapping them to the host's LAN IP address.
type Responder struct {
	port   int
	lanIP  string
	mu     sync.RWMutex
	names  map[string]bool
	logger *slog.Logger
	server *mdns.Server
}

// NewResponder creates a new mDNS responder.
func NewResponder(logger *slog.Logger) (*Responder, error) {
	if logger == nil {
		logger = slog.Default()
	}
	lanIP, err := GetLANIP()
	if err != nil {
		return nil, err
	}
	return &Responder{
		port:   5353,
		lanIP:  lanIP,
		names:  make(map[string]bool),
		logger: logger,
	}, nil
}

// Announce adds a name to the mDNS responder.
func (r *Responder) Announce(name string) {
	r.mu.Lock()
	r.names[strings.ToLower(name)] = true
	r.mu.Unlock()
	r.logger.Info("mDNS announcing", "name", name+".local", "ip", r.lanIP)
}

// Withdraw removes a name from the mDNS responder.
func (r *Responder) Withdraw(name string) {
	r.mu.Lock()
	delete(r.names, strings.ToLower(name))
	r.mu.Unlock()
}

// Start begins the mDNS responder (listens on udp 224.0.0.251:5353).
func (r *Responder) Start(ctx context.Context) error {
	handler := mdns.NewServeMux()
	handler.HandleFunc("local.", r.handleQuery)

	addr := &net.UDPAddr{
		IP:   net.ParseIP("224.0.0.251"),
		Port: r.port,
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		// Fallback: non-multicast on localhost for testing / permission issues
		r.logger.Warn("could not bind multicast, falling back to unicast", "error", err)
		r.server = &mdns.Server{
			Addr:    fmt.Sprintf(":%d", r.port),
			Net:     "udp",
			Handler: handler,
		}
		go func() {
			<-ctx.Done()
			r.server.Shutdown()
		}()
		return r.server.ListenAndServe()
	}

	r.server = &mdns.Server{
		PacketConn: conn,
		Handler:    handler,
	}

	go func() {
		<-ctx.Done()
		r.server.Shutdown()
	}()

	return r.server.ActivateAndServe()
}

// Stop shuts down the mDNS responder.
func (r *Responder) Stop() {
	if r.server != nil {
		r.server.Shutdown()
	}
}

func (r *Responder) handleQuery(w mdns.ResponseWriter, msg *mdns.Msg) {
	resp := new(mdns.Msg)
	resp.SetReply(msg)
	resp.Authoritative = true

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, q := range msg.Question {
		name := strings.ToLower(q.Name)
		// Strip trailing dot and ".local."
		stripped := strings.TrimSuffix(name, ".")
		stripped = strings.TrimSuffix(stripped, ".local")

		if !r.names[stripped] {
			continue
		}

		switch q.Qtype {
		case mdns.TypeA:
			resp.Answer = append(resp.Answer, &mdns.A{
				Hdr: mdns.RR_Header{
					Name:   q.Name,
					Rrtype: mdns.TypeA,
					Class:  mdns.ClassINET,
					Ttl:    120,
				},
				A: net.ParseIP(r.lanIP),
			})
		case mdns.TypeAAAA:
			// Only respond with A records for LAN sharing
		}
	}

	if len(resp.Answer) > 0 {
		w.WriteMsg(resp)
	}
}

// GetLANIP returns the local network IP address.
func GetLANIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("getting interface addrs: %w", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.To4() == nil || ip.IsLinkLocalUnicast() {
			continue
		}
		return ip.String(), nil
	}
	return "", fmt.Errorf("no LAN IP found")
}

// ShareInfo returns LAN sharing URLs for a route.
func ShareInfo(name string, proxyPort int, logger *slog.Logger) (*LANInfo, error) {
	lanIP, err := GetLANIP()
	if err != nil {
		return nil, err
	}
	return &LANInfo{
		LocalIP:  lanIP,
		LocalURL: fmt.Sprintf("http://%s.localhost:%d", name, proxyPort),
		LANURL:   fmt.Sprintf("http://%s.local:%d", name, proxyPort),
	}, nil
}
