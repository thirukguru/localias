// Package dns provides a local DNS server resolving *.localhost → 127.0.0.1.
// Uses the miekg/dns library for DNS protocol handling.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/dns
package dns

import (
	"fmt"
	"log/slog"
	"net"
	"strings"

	mdns "github.com/miekg/dns"
)

// Server is a local DNS server for .localhost resolution.
type Server struct {
	port   int
	logger *slog.Logger
	server *mdns.Server
}

// NewServer creates a new DNS server.
func NewServer(port int, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if port == 0 {
		port = 5533
	}
	return &Server{
		port:   port,
		logger: logger,
	}
}

// Start begins the DNS server.
func (s *Server) Start() error {
	handler := mdns.NewServeMux()
	handler.HandleFunc("localhost.", s.handleLocalhost)

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	s.server = &mdns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: handler,
	}

	s.logger.Info("DNS server started", "addr", addr)
	return s.server.ListenAndServe()
}

// Stop shuts down the DNS server.
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown()
	}
	return nil
}

func (s *Server) handleLocalhost(w mdns.ResponseWriter, r *mdns.Msg) {
	msg := new(mdns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		name := strings.ToLower(q.Name)
		if !strings.HasSuffix(name, "localhost.") {
			continue
		}

		switch q.Qtype {
		case mdns.TypeA:
			msg.Answer = append(msg.Answer, &mdns.A{
				Hdr: mdns.RR_Header{
					Name:   q.Name,
					Rrtype: mdns.TypeA,
					Class:  mdns.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP("127.0.0.1"),
			})
		case mdns.TypeAAAA:
			msg.Answer = append(msg.Answer, &mdns.AAAA{
				Hdr: mdns.RR_Header{
					Name:   q.Name,
					Rrtype: mdns.TypeAAAA,
					Class:  mdns.ClassINET,
					Ttl:    60,
				},
				AAAA: net.ParseIP("::1"),
			})
		}
	}

	if err := w.WriteMsg(msg); err != nil {
		s.logger.Debug("failed to write DNS response", "error", err)
	}
}
