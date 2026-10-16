// Package ai calls the site's language-model provider: an OpenAI-compatible
// Responses API for answers (text, tool calls, structured JSON, images in the
// input) and its audio endpoint for transcriptions. `ddcore.ai` in server
// code and the framework's own features (the Feedback assistant) go through
// it, so retries, time budgets, error wording and what is logged are decided
// once.
//
// Nothing here logs a prompt, an answer or the key: a prompt carries customer
// text, and a log is read by more people than the database.
package ai

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
)

// DefaultTimeout bounds one Respond, retries included, when neither the call
// nor the site names a timeout.
const DefaultTimeout = 45 * time.Second

// DefaultTranscribeTimeout bounds one Transcribe when the call names none:
// an upload of a few minutes of audio takes longer than a text answer.
const DefaultTranscribeTimeout = 120 * time.Second

// Config is what a Client needs from the site's `ai` configuration.
type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	TranscriptionModel string
	ReasoningEffort    string
	Timeout            time.Duration
}

// FromSite turns the site's configuration into a Client's. A site that turned
// the AI off gets a Config with no key, which every call refuses.
func FromSite(c config.AI) Config {
	cfg := Config{
		BaseURL: c.URL(), Model: c.Model, TranscriptionModel: c.TranscriptionModel,
		ReasoningEffort: c.ReasoningEffort, Timeout: time.Duration(c.TimeoutSeconds) * time.Second,
	}
	if c.Enabled == nil || *c.Enabled {
		cfg.APIKey = c.APIKey
	}
	return cfg
}

// Client calls one provider. It is safe for concurrent use.
type Client struct {
	cfg Config
	// HTTP sends the requests; nil is a client with no timeout of its own,
	// since every request carries its own deadline.
	HTTP *http.Client
	// Log receives one line per attempt: kind, model, status, latency, tokens.
	Log *slog.Logger
	// sleep waits between attempts; tests make it a no-op.
	sleep func(time.Duration)
}

// New builds a Client. A nil log is slog.Default().
func New(cfg Config, log *slog.Logger) *Client {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = config.DefaultAIBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{cfg: cfg, HTTP: &http.Client{}, Log: log, sleep: time.Sleep}
}

// Enabled is whether a call can be made at all: a key and a default model.
func (c *Client) Enabled() bool { return c.cfg.APIKey != "" && c.cfg.Model != "" }

// CanTranscribe is whether Transcribe has a model to call without being told one.
func (c *Client) CanTranscribe() bool { return c.cfg.APIKey != "" && c.cfg.TranscriptionModel != "" }

// budget is the time a call may take: its own timeout in seconds, else def.
func budget(seconds float64, def time.Duration) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	return def
}
