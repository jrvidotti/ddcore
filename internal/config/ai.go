package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// DefaultAIBaseURL is where an OpenAI-compatible Responses API answers when
// the site names no other.
const DefaultAIBaseURL = "https://api.openai.com/v1"

// DefaultAIModel and DefaultAITranscriptionModel answer when the site names
// no model: with the default address, a key is all a site needs.
const (
	DefaultAIModel              = "gpt-6-luna"
	DefaultAITranscriptionModel = "gpt-transcribe"
)

// AI is the site's language-model provider, an OpenAI-compatible Responses
// API that `ddcore.ai` and the framework's own features (the Feedback
// assistant) call. Which model answers is the site's decision, so it lives in
// ddcore.json; the key is a credential and comes from DDCORE_AI_API_KEY only;
// the address and the rest may differ per deployment, so the environment
// overrides the file.
type AI struct {
	// Enabled is nil when ddcore.json says nothing, which means on as soon as
	// a key is set.
	Enabled *bool `json:"enabled,omitempty"`
	// BaseURL is the API's root, DefaultAIBaseURL when empty.
	BaseURL string `json:"baseUrl,omitempty"`
	// Model answers `ddcore.ai.respond` when the call names none;
	// DefaultAIModel when empty.
	Model string `json:"model,omitempty"`
	// TranscriptionModel answers `ddcore.ai.transcribe` when the call names
	// none; DefaultAITranscriptionModel when empty.
	TranscriptionModel string `json:"transcriptionModel,omitempty"`
	// ReasoningEffort is sent with every respond that names none; empty, none is sent.
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// TimeoutSeconds bounds one respond, retries included, when the call
	// names no timeout. 0 is 45.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// APIKey is DDCORE_AI_API_KEY: never in a committed file.
	APIKey string `json:"-"`
}

// On is whether the site can call a model: a key, and not turned off.
func (a AI) On() bool { return (a.Enabled == nil || *a.Enabled) && a.APIKey != "" }

// CanTranscribe is whether the site can also turn audio into text.
func (a AI) CanTranscribe() bool { return a.On() }

// URL is the API's root without a trailing slash.
func (a AI) URL() string {
	if u := strings.TrimRight(strings.TrimSpace(a.BaseURL), "/"); u != "" {
		return u
	}
	return DefaultAIBaseURL
}

// fromEnv applies DDCORE_AI (on/off), DDCORE_AI_API_KEY and the DDCORE_AI_*
// overrides of the file's keys, then the default models.
func (a *AI) fromEnv() {
	if strings.TrimSpace(os.Getenv("DDCORE_AI")) != "" {
		on := envBool("DDCORE_AI", a.Enabled == nil || *a.Enabled)
		a.Enabled = &on
	}
	a.APIKey = env("DDCORE_AI_API_KEY", "")
	a.BaseURL = env("DDCORE_AI_BASE_URL", a.BaseURL)
	a.Model = env("DDCORE_AI_MODEL", a.Model)
	a.TranscriptionModel = env("DDCORE_AI_TRANSCRIPTION_MODEL", a.TranscriptionModel)
	a.ReasoningEffort = env("DDCORE_AI_REASONING_EFFORT", a.ReasoningEffort)
	if a.Model == "" {
		a.Model = DefaultAIModel
	}
	if a.TranscriptionModel == "" {
		a.TranscriptionModel = DefaultAITranscriptionModel
	}
	if v := env("DDCORE_AI_TIMEOUT_SECONDS", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.TimeoutSeconds = n
		} else {
			a.TimeoutSeconds = -1 // validate names the variable
		}
	}
}

// validate refuses an address that is not http(s) and a timeout out of range:
// either would only show as a failed call at the worst moment.
func (a AI) validate() error {
	if a.BaseURL != "" {
		u, err := url.Parse(strings.TrimSpace(a.BaseURL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("ai.baseUrl: %q is not an http(s) address", a.BaseURL)
		}
	}
	if a.TimeoutSeconds < 0 || a.TimeoutSeconds > 600 {
		return fmt.Errorf("ai.timeoutSeconds (DDCORE_AI_TIMEOUT_SECONDS) must be a number between 0 and 600")
	}
	return nil
}
