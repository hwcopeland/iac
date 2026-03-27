// cmd/kai-agent/main.go — Phase 2 kai-agent stub.
//
// Reads identity and callback coordinates from environment variables,
// simulates work, and POSTs a completion callback to kai-api's internal
// endpoint.  No external dependencies: stdlib only.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// env returns the value of the named environment variable, exiting with a
// clear error message if the variable is not set.
func env(name string) string {
	v := os.Getenv(name)
	if v == "" {
		slog.Error("required env var not set", "var", name)
		os.Exit(1)
	}
	return v
}

// callbackPayload mirrors the JSON body expected by
// POST /internal/callback on kai-api.
//
// taskID is intentionally empty for Phase 2; the orchestrator resolves the
// task by (runID, agentRole).
type callbackPayload struct {
	TaskID  string `json:"taskId"`
	RunID   string `json:"runId"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func main() {
	// ── 1. Read environment ───────────────────────────────────────────────────
	runID       := env("KAI_RUN_ID")
	teamID      := env("KAI_TEAM_ID")
	agentRole   := env("KAI_AGENT_ROLE")
	sandboxName := env("KAI_SANDBOX_NAME")
	callbackURL := env("KAI_CALLBACK_URL")
	callbackTok := env("KAI_CALLBACK_TOKEN")

	// ── 2. Log startup ────────────────────────────────────────────────────────
	slog.Info("kai-agent starting",
		"role",        agentRole,
		"runID",       runID,
		"teamID",      teamID,
		"sandbox",     sandboxName,
		"callbackURL", callbackURL,
	)

	// ── 3. Simulate work ──────────────────────────────────────────────────────
	slog.Info("kai-agent working", "role", agentRole)
	time.Sleep(5 * time.Second)
	slog.Info("kai-agent work complete", "role", agentRole)

	// ── 4. POST completion callback ───────────────────────────────────────────
	payload := callbackPayload{
		TaskID:  "",
		RunID:   runID,
		Status:  "succeeded",
		Message: fmt.Sprintf("Agent %s completed", agentRole),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal callback payload", "err", err)
		os.Exit(1)
	}

	req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to build callback request", "err", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kai-Callback-Token", callbackTok)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("callback POST failed", "url", callbackURL, "err", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("callback returned non-2xx", "status", resp.StatusCode, "url", callbackURL)
		os.Exit(1)
	}

	slog.Info("kai-agent callback accepted", "role", agentRole, "status", resp.StatusCode)
}
