// cmd/kai-agent/main.go — Kai agent: calls xAI Grok API to execute a role in a team run.
//
// Each agent reads its role, the run objective, and its xAI API key from env,
// calls grok-3-fast via the OpenAI-compatible xAI API, and POSTs the result
// back to kai-api's internal callback endpoint.
package main

import (
"bytes"
"encoding/json"
"fmt"
"io"
"log/slog"
"net/http"
"os"
"time"
)

const (
xaiBaseURL = "https://api.x.ai/v1/chat/completions"
grokModel  = "grok-3-fast"
)

// rolePrompts define what each agent role does with the run objective.
var rolePrompts = map[string]string{
"planner":    "You are a project planner. Break down the following objective into a clear, numbered implementation plan with concrete tasks. Be specific and actionable.",
"researcher": "You are a research assistant. Investigate the following objective and provide relevant background, existing solutions, libraries, or approaches the team should be aware of.",
"coder_1":    "You are a senior software engineer. Implement the core logic for the following objective. Write clean, well-commented code.",
"coder_2":    "You are a senior software engineer. Build supporting components, tests, or integrations for the following objective. Complement the core implementation.",
"reviewer":   "You are a code reviewer. Review the work done on the following objective. Identify issues, suggest improvements, and confirm the implementation meets the goal.",
}

func env(name string) string {
v := os.Getenv(name)
if v == "" {
slog.Error("required env var not set", "var", name)
os.Exit(1)
}
return v
}

type xaiMessage struct {
Role    string `json:"role"`
Content string `json:"content"`
}

type xaiRequest struct {
Model    string       `json:"model"`
Messages []xaiMessage `json:"messages"`
}

type xaiResponse struct {
Choices []struct {
Message xaiMessage `json:"message"`
} `json:"choices"`
Error *struct {
Message string `json:"message"`
} `json:"error,omitempty"`
}

type callbackPayload struct {
TaskID  string `json:"taskId"`
RunID   string `json:"runId"`
Status  string `json:"status"`
Message string `json:"message"`
}

func callGrok(apiKey, systemPrompt, objective string) (string, error) {
reqBody := xaiRequest{
Model: grokModel,
Messages: []xaiMessage{
{Role: "system", Content: systemPrompt},
{Role: "user", Content: objective},
},
}
body, err := json.Marshal(reqBody)
if err != nil {
return "", fmt.Errorf("marshal request: %w", err)
}
req, err := http.NewRequest(http.MethodPost, xaiBaseURL, bytes.NewReader(body))
if err != nil {
return "", fmt.Errorf("build request: %w", err)
}
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+apiKey)

client := &http.Client{Timeout: 5 * time.Minute}
resp, err := client.Do(req)
if err != nil {
return "", fmt.Errorf("xAI API call: %w", err)
}
defer resp.Body.Close()

respBytes, err := io.ReadAll(resp.Body)
if err != nil {
return "", fmt.Errorf("read response: %w", err)
}
var xaiResp xaiResponse
if err := json.Unmarshal(respBytes, &xaiResp); err != nil {
return "", fmt.Errorf("unmarshal response: %w", err)
}
if xaiResp.Error != nil {
return "", fmt.Errorf("xAI API error: %s", xaiResp.Error.Message)
}
if len(xaiResp.Choices) == 0 {
return "", fmt.Errorf("xAI API returned no choices (status %d)", resp.StatusCode)
}
return xaiResp.Choices[0].Message.Content, nil
}

func postCallback(callbackURL, callbackToken, runID, status, message string) error {
payload := callbackPayload{RunID: runID, Status: status, Message: message}
body, err := json.Marshal(payload)
if err != nil {
return err
}
req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body))
if err != nil {
return err
}
req.Header.Set("Content-Type", "application/json")
req.Header.Set("X-Kai-Callback-Token", callbackToken)

client := &http.Client{Timeout: 15 * time.Second}
resp, err := client.Do(req)
if err != nil {
return err
}
defer resp.Body.Close()
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
return fmt.Errorf("callback returned %d", resp.StatusCode)
}
return nil
}

func main() {
runID       := env("KAI_RUN_ID")
agentRole   := env("KAI_AGENT_ROLE")
callbackURL := env("KAI_CALLBACK_URL")
callbackTok := env("KAI_CALLBACK_TOKEN")
xaiKey      := env("XAI_API_KEY")
objective   := os.Getenv("KAI_OBJECTIVE")
if objective == "" {
objective = "Build a well-structured solution following best practices."
}

slog.Info("kai-agent starting", "role", agentRole, "runID", runID, "model", grokModel)

systemPrompt, ok := rolePrompts[agentRole]
if !ok {
systemPrompt = "You are a helpful AI assistant. Complete the following task thoroughly."
}

result, err := callGrok(xaiKey, systemPrompt, objective)
if err != nil {
slog.Error("grok call failed", "role", agentRole, "err", err)
_ = postCallback(callbackURL, callbackTok, runID, "failed", err.Error())
os.Exit(1)
}

slog.Info("kai-agent grok call succeeded", "role", agentRole, "chars", len(result))

if err := postCallback(callbackURL, callbackTok, runID, "succeeded", result); err != nil {
slog.Error("callback failed", "role", agentRole, "err", err)
os.Exit(1)
}

slog.Info("kai-agent complete", "role", agentRole)
}
