package events

import "time"

// RunEvent represents a single event published to the EventHub for a run.
// Payload is a raw JSON string so callers can embed arbitrary event-specific data
// without a second unmarshal pass.
type RunEvent struct {
	ID          string    `json:"id"`
	RunID       string    `json:"runId"`
	AgentTaskID *string   `json:"agentTaskId,omitempty"`
	EventType   string    `json:"eventType"`
	Payload     string    `json:"payload"` // raw JSON string
	CreatedAt   time.Time `json:"createdAt"`
}
