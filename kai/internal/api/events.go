package api

import "net/http"

// GET /api/runs/{runID}/events
//
// WebSocket event stream — Phase 3 only.
// Returns 501 Not Implemented until the WebSocket upgrade path is built.
func (s *Server) handleRunEvents(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "WebSocket event stream not yet implemented (Phase 3)", http.StatusNotImplemented)
}

