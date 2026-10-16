package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// Schema asks for an answer that is JSON matching it (the Responses API's
// `text.format` of type `json_schema`).
type Schema struct {
	Name        string          `json:"name"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"` // nil is true
	Description string          `json:"description,omitempty"`
}

// Request is one call to the Responses API. Input and Tools are passed through
// as they are, so the items of an earlier answer (encrypted reasoning, function
// calls) can be replayed untouched.
type Request struct {
	// Model is "" for the site's default.
	Model        string `json:"model,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	// Input is a string or an array of input items.
	Input      json.RawMessage `json:"input,omitempty"`
	Tools      json.RawMessage `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"toolChoice,omitempty"`
	Schema     *Schema         `json:"schema,omitempty"`
	// ReasoningEffort is "" for the site's default; still "", none is sent.
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
	// IncludeReasoning is nil for true: ask for the encrypted reasoning, which
	// a tool loop replays. A provider that refuses `include` takes false.
	IncludeReasoning *bool `json:"includeReasoning,omitempty"`
	// Timeout is the whole call in seconds, retries included; 0 is the site's.
	Timeout float64 `json:"timeout,omitempty"`
}

// FunctionCall is a tool the model asked to run.
type FunctionCall struct {
	CallID    string `json:"callId"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage is what the call cost.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// Response is the provider's answer, read.
type Response struct {
	ID               string            `json:"id"`
	Model            string            `json:"model"`
	Status           string            `json:"status"`
	IncompleteReason string            `json:"incompleteReason"`
	Output           []json.RawMessage `json:"output"`
	Text             string            `json:"text"`
	Calls            []FunctionCall    `json:"calls"`
	JSON             any               `json:"json"`
	Usage            Usage             `json:"usage"`
}

// Prepare is the request body Respond sends, and the time it may take. It is
// pure, so the test fake of `ddcore.ai` builds exactly what the real call would.
func (c *Client) Prepare(req Request) (url string, body map[string]any, total time.Duration, err error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.cfg.Model
	}
	if model == "" {
		return "", nil, 0, cerr.Validation("No AI model: set ai.model in ddcore.json or pass model")
	}
	body = map[string]any{"model": model, "store": false}
	if req.Instructions != "" {
		body["instructions"] = req.Instructions
	}
	if len(req.Input) > 0 && string(req.Input) != "null" {
		body["input"] = req.Input
	} else {
		return "", nil, 0, cerr.Validation("ddcore.ai.respond needs an input")
	}
	if req.IncludeReasoning == nil || *req.IncludeReasoning {
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	if t := bytes.TrimSpace(req.Tools); len(t) > 0 && string(t) != "null" && string(t) != "[]" {
		body["tools"] = req.Tools
	}
	if t := bytes.TrimSpace(req.ToolChoice); len(t) > 0 && string(t) != "null" {
		body["tool_choice"] = req.ToolChoice
	}
	effort := req.ReasoningEffort
	if effort == "" {
		effort = c.cfg.ReasoningEffort
	}
	if effort != "" {
		body["reasoning"] = map[string]any{"effort": effort}
	}
	if req.MaxOutputTokens > 0 {
		body["max_output_tokens"] = req.MaxOutputTokens
	}
	if s := req.Schema; s != nil {
		if len(s.Schema) == 0 {
			return "", nil, 0, cerr.Validation("ddcore.ai.respond: schema needs a schema")
		}
		name := s.Name
		if name == "" {
			name = "answer"
		}
		format := map[string]any{"type": "json_schema", "name": name, "schema": s.Schema, "strict": s.Strict == nil || *s.Strict}
		if s.Description != "" {
			format["description"] = s.Description
		}
		body["text"] = map[string]any{"format": format}
	}
	return c.cfg.BaseURL + "/responses", body, budget(req.Timeout, c.cfg.Timeout), nil
}

// Respond calls the model and reads its answer.
func (c *Client) Respond(ctx context.Context, req Request) (*Response, error) {
	if c.cfg.APIKey == "" {
		return nil, notConfigured()
	}
	url, body, total, err := c.Prepare(req)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, cerr.Validation("ddcore.ai.respond: {0}", err.Error())
	}
	status, resp, err := c.do(ctx, "respond", body["model"].(string), total, func(ctx context.Context) (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	r, err := Parse(status, resp, req.Schema != nil)
	if err == nil {
		c.Log.Info("ai usage", "kind", "respond", "model", r.Model,
			"inputTokens", r.Usage.InputTokens, "outputTokens", r.Usage.OutputTokens)
	}
	return r, err
}

// Parse reads a Responses API answer. wantJSON makes an answer that was cut
// short or is not JSON an error, since the caller asked for structure.
func Parse(status int, raw []byte, wantJSON bool) (*Response, error) {
	if status != http.StatusOK {
		return nil, providerError(status, raw)
	}
	var data struct {
		ID                string            `json:"id"`
		Model             string            `json:"model"`
		Status            string            `json:"status"`
		Output            []json.RawMessage `json:"output"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, cerr.Unavailable("AI request failed: {0}", "the answer is not JSON")
	}
	if data.Status == "failed" {
		msg := "failed"
		if data.Error != nil && data.Error.Message != "" {
			msg = cut(data.Error.Message, 300)
		}
		return nil, cerr.Unavailable("AI request failed: {0}", msg)
	}
	if data.Output == nil {
		return nil, cerr.Unavailable("AI request failed: {0}", "the answer has no output")
	}
	r := &Response{
		ID: data.ID, Model: data.Model, Status: data.Status, Output: data.Output,
		Calls: []FunctionCall{}, Usage: Usage{data.Usage.InputTokens, data.Usage.OutputTokens},
	}
	if data.IncompleteDetails != nil {
		r.IncompleteReason = data.IncompleteDetails.Reason
	}
	var text []string
	for _, item := range data.Output {
		var it struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(item, &it) != nil {
			continue
		}
		switch it.Type {
		case "message":
			for _, p := range it.Content {
				if p.Type == "output_text" && p.Text != "" {
					text = append(text, p.Text)
				}
			}
		case "function_call":
			r.Calls = append(r.Calls, FunctionCall{CallID: it.CallID, Name: it.Name, Arguments: it.Arguments})
		}
	}
	r.Text = strings.Join(text, "\n")
	if wantJSON {
		if r.Status == "incomplete" {
			return nil, cerr.Unavailable("The AI answer was cut short: {0}", r.IncompleteReason)
		}
		if err := json.Unmarshal([]byte(r.Text), &r.JSON); err != nil {
			return nil, cerr.Unavailable("The AI answered with invalid JSON")
		}
	}
	return r, nil
}

// providerError words a status that is not 200: the provider's own message
// when it sent its error JSON, else only the status. A raw body is never
// echoed, since a proxy's error page can quote the request.
func providerError(status int, raw []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := fmt.Sprintf("HTTP %d", status)
	if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
		msg += ": " + cut(e.Error.Message, 300)
	}
	if transient(status) {
		return cerr.Unavailable("AI request failed: {0}", msg)
	}
	return cerr.Validation("AI request failed: {0}", msg)
}

func notConfigured() error {
	return cerr.Validation("AI is not configured on this site").WithTitleKey("AI")
}

// cut keeps s under n runes, marking the cut.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
