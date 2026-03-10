// Package proxy — WebSocket proxying support.
// Detects Upgrade: websocket headers and proxies the connection using HTTP hijacking.
// Required for Next.js HMR, Vite HMR, and other WebSocket-based dev tools.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request.
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, v := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(v), "upgrade") {
			return true
		}
	}
	return false
}

// proxyWebSocket hijacks the client connection and establishes a bidirectional
// pipe between the client and the backend WebSocket server.
func (h *Handler) proxyWebSocket(w http.ResponseWriter, r *http.Request, route *Route) {
	// Connect to backend
	backendAddr := fmt.Sprintf("127.0.0.1:%d", route.Port)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		h.logger.Error("websocket backend dial failed", "route", route.Name, "error", err)
		http.Error(w, "WebSocket backend unavailable", http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		h.logger.Error("hijacking not supported")
		http.Error(w, "WebSocket hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		h.logger.Error("hijack failed", "error", err)
		http.Error(w, "WebSocket hijack failed", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Forward the original request to the backend
	r.Header.Set("X-Localias-Forwarded", "true")
	if err := r.Write(backendConn); err != nil {
		h.logger.Error("websocket request forward failed", "error", err)
		return
	}

	h.logger.Info("websocket connected", "route", route.Name, "path", r.URL.Path)

	// Bidirectional copy
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errCh <- err
	}()

	// Wait for either direction to finish
	<-errCh

	h.logger.Info("websocket disconnected", "route", route.Name)
}

// isClosedError checks if an error is a closed connection error.
func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe")
}
