// Package proxy — loop detection for the reverse proxy.
// If an incoming request to a registered route already has the X-Localias-Forwarded
// header set, it means the request is looping back through the proxy.
// Returns a 508 Loop Detected response with a JSON error body.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/proxy
package proxy

import (
	"encoding/json"
	"net/http"
)

// LoopError is the JSON response body for loop detection.
type LoopError struct {
	Error string `json:"error"`
	Fix   string `json:"fix"`
}

// handleLoop writes a 508 Loop Detected response with a JSON body.
func handleLoop(w http.ResponseWriter, routeName string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusLoopDetected) // 508

	resp := LoopError{
		Error: "loop_detected",
		Fix:   "add changeOrigin: true to your proxy config, or remove the proxy middleware that forwards to localias",
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck // best-effort after status code is set
}

// isLoopRequest checks if a request has the loop detection header set.
func isLoopRequest(r *http.Request) bool {
	return r.Header.Get("X-Localias-Forwarded") != ""
}
