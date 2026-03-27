package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/hwcopeland/iac/kai/internal/auth"
)

const (
	// wsWriteWait is the maximum time allowed to write a single message to the peer.
	wsWriteWait = 10 * time.Second
	// wsPongWait is the maximum time to wait for a pong after the last ping.
	// If no pong is received within this window the connection is closed.
	wsPongWait = 30 * time.Second
	// wsPingInterval controls how frequently the server sends WebSocket pings.
	// Must be less than wsPongWait.
	wsPingInterval = 15 * time.Second
)

// wsUpgrader is the gorilla/websocket upgrader shared by all event-stream connections.
// CheckOrigin allows same-origin browser clients and non-browser tool clients (empty Origin).
var wsUpgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Non-browser clients (curl, server-side) do not set Origin.
			return true
		}
		return origin == "https://kai.hwcopeland.net"
	},
}

// GET /api/runs/{runID}/events
//
// Upgrades the HTTP connection to WebSocket and streams run events as newline-
// delimited JSON objects. Requires an authenticated session (enforced by
// SessionMiddleware upstream, but re-checked here before the upgrade so that
// a 401 can be returned before headers are hijacked).
//
// Protocol:
//   - On connect the hub replays buffered history (up to 2000 events) through
//     the subscription channel before forwarding live events.
//   - The server sends a WebSocket ping frame every 15 s.
//   - If no pong is received within 30 s the server closes the connection.
//   - A 10 s write deadline is enforced per message.
//   - Each message is the JSON serialisation of events.RunEvent.
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	// ── Auth guard ────────────────────────────────────────────────────────────
	// SessionMiddleware enforces auth before we are called, but we must
	// verify before upgrading — once headers are hijacked we cannot send 401.
	_, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	runID := chi.URLParam(r, "runID")

	// ── WebSocket upgrade ─────────────────────────────────────────────────────
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrader has already written an error response; just log.
		slog.Warn("handleRunEvents: websocket upgrade failed",
			"runID", runID, "err", err)
		return
	}
	defer conn.Close()

	// ── Connection-scoped context ─────────────────────────────────────────────
	// Cancelled when the read pump detects a disconnect or pong timeout.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// ── Subscribe ─────────────────────────────────────────────────────────────
	// hub.Subscribe snapshots history and registers the live-event channel
	// atomically. History is replayed into ch by an internal goroutine before
	// live events arrive, so no separate hub.History call is needed here.
	ch, unsub := s.hub.Subscribe(ctx, runID)
	defer unsub()

	// ── Pong / read-deadline handling ─────────────────────────────────────────
	// SetReadDeadline must be primed before the read pump starts.
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		// Each pong extends the deadline by the full pongWait window.
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	// Read pump — gorilla/websocket requires that we read all incoming frames
	// to keep pong processing active. Runs until the connection errors out
	// (disconnect, deadline exceeded, or explicit close), at which point it
	// cancels the shared context to wake the write loop.
	go func() {
		defer cancel()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// ── Write loop ────────────────────────────────────────────────────────────
	pingTicker := time.NewTicker(wsPingInterval)
	defer pingTicker.Stop()

	for {
		select {

		case event, open := <-ch:
			// open is always true (hub never closes channels), but guard anyway
			// in case the Hub implementation changes.
			if !open {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteJSON(event); err != nil {
				slog.Debug("handleRunEvents: write error",
					"runID", runID, "err", err)
				return
			}

		case <-pingTicker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				slog.Debug("handleRunEvents: ping error",
					"runID", runID, "err", err)
				return
			}

		case <-ctx.Done():
			// Client disconnected or pong deadline exceeded — read pump already
			// fired cancel(); exit cleanly and let defer conn.Close() send the
			// close frame.
			return
		}
	}
}

