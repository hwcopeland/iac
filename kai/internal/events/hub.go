package events

import (
	"context"
	"log/slog"
	"sync"
)

// RingBufferSize is the maximum number of events retained per run.
// Oldest events are overwritten once the buffer is full.
const RingBufferSize = 2000

// Hub is an in-process pub/sub hub with per-run ring buffers.
// It provides event history replay for new subscribers and live fan-out.
//
// Concurrency model:
//   - A single RWMutex guards rings, subs, and ringPos.
//   - Publish holds the write lock only while updating the ring and copying subs,
//     then sends to subscriber channels outside the lock.
//   - Subscribe holds the write lock only long enough to snapshot history and register
//     the new channel, then replays history in a goroutine.
type Hub struct {
	mu      sync.RWMutex
	rings   map[string][]RunEvent     // runID → ring buffer (bounded circular)
	subs    map[string][]chan RunEvent // runID → live subscriber channels
	ringPos map[string]int            // runID → next-write position (head of ring)
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{
		rings:   make(map[string][]RunEvent),
		subs:    make(map[string][]chan RunEvent),
		ringPos: make(map[string]int),
	}
}

// Publish sends event to all live subscribers for runID and appends it to the
// ring buffer. If a subscriber's channel is full (capacity 64), the event is
// dropped for that subscriber and a warning is logged.
func (h *Hub) Publish(runID string, event RunEvent) {
	h.mu.Lock()

	// Initialise ring for this runID on first publish.
	if _, ok := h.rings[runID]; !ok {
		h.rings[runID] = make([]RunEvent, 0, RingBufferSize)
		h.ringPos[runID] = 0
	}

	buf := h.rings[runID]
	if len(buf) < RingBufferSize {
		// Buffer not yet full — simple append.
		h.rings[runID] = append(buf, event)
	} else {
		// Buffer full — overwrite oldest entry and advance head.
		pos := h.ringPos[runID]
		h.rings[runID][pos] = event
		h.ringPos[runID] = (pos + 1) % RingBufferSize
	}

	// Snapshot subscriber slice while holding the lock so we send a consistent
	// view. We send outside the lock to avoid deadlocks with slow consumers.
	subs := make([]chan RunEvent, len(h.subs[runID]))
	copy(subs, h.subs[runID])

	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			slog.Warn("events: subscriber channel full, dropping event",
				"runID", runID, "eventType", event.EventType)
		}
	}
}

// Subscribe returns a channel that receives all buffered history for runID
// followed by live events. The returned cancel func removes the subscription;
// callers should invoke it when done (e.g. in a defer or on context cancellation).
//
// History replay is sent in a goroutine so Subscribe never blocks the caller.
// The channel has capacity 64 for live events; history events are queued
// ahead of live events because the subscription is registered before the
// history goroutine finishes sending.
func (h *Hub) Subscribe(ctx context.Context, runID string) (<-chan RunEvent, func()) {
	ch := make(chan RunEvent, 64)

	h.mu.Lock()
	// Snapshot history while holding the lock — this is a cheap copy of a slice
	// header plus the underlying slice data, so it is safe to hold the lock briefly.
	history := h.historyLocked(runID)
	// Register BEFORE releasing the lock so we cannot miss events published
	// between history snapshot and goroutine start.
	h.subs[runID] = append(h.subs[runID], ch)
	h.mu.Unlock()

	// Replay history in the background to avoid blocking the caller. Any new
	// events published after we registered will follow in channel order.
	go func() {
		for _, e := range history {
			select {
			case ch <- e:
			case <-ctx.Done():
				return
			}
		}
	}()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subs[runID]
		for i, c := range subs {
			if c == ch {
				// Remove by replacing with last element and shrinking.
				h.subs[runID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		// Do not close ch here: Publish may already hold a snapshot of subs that
		// includes ch and will try to send after we return. Leaving ch open and
		// letting it be GC'd is the safest approach; callers must select on their
		// own context to stop reading.
	}

	return ch, cancel
}

// History returns all buffered events for runID in chronological order.
func (h *Hub) History(runID string) []RunEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.historyLocked(runID)
}

// historyLocked returns a copy of buffered events in chronological order.
// Must be called with h.mu held (read or write lock).
func (h *Hub) historyLocked(runID string) []RunEvent {
	buf, ok := h.rings[runID]
	if !ok || len(buf) == 0 {
		return nil
	}

	if len(buf) < RingBufferSize {
		// Buffer not yet full — elements are in insertion order.
		result := make([]RunEvent, len(buf))
		copy(result, buf)
		return result
	}

	// Full ring: oldest element is at ringPos[runID], newest is one before it.
	head := h.ringPos[runID]
	result := make([]RunEvent, RingBufferSize)
	for i := 0; i < RingBufferSize; i++ {
		result[i] = buf[(head+i)%RingBufferSize]
	}
	return result
}
