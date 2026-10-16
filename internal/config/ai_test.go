package config

import (
	"strings"
	"testing"
)

func TestAIOffWithoutKey(t *testing.T) {
	clearMailEnv(t)
	f, _, err := Load(site(t, `{"ai":{"model":"m1"}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.AI.On() {
		t.Error("a model without a key is off")
	}
	if f.AI.URL() != DefaultAIBaseURL {
		t.Errorf("default base URL, got %q", f.AI.URL())
	}
	if f.Feedback.AssistOn(f.AI) {
		t.Error("the assistant needs the AI")
	}

	t.Setenv("DDCORE_AI_API_KEY", "sk-test")
	if f, _, err = Load(site(t, `{}`)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !f.AI.On() || f.AI.Model != DefaultAIModel || f.AI.TranscriptionModel != DefaultAITranscriptionModel {
		t.Errorf("a key is enough: the default models answer, got %+v", f.AI)
	}
}

func TestAIFromFileAndEnv(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_AI_API_KEY", "sk-test")
	t.Setenv("DDCORE_AI_MODEL", "env-model")
	t.Setenv("DDCORE_AI_BASE_URL", "https://llm.example.com/v1/")
	f, _, err := Load(site(t, `{"ai":{"model":"file-model","transcriptionModel":"t1","timeoutSeconds":60}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !f.AI.On() || !f.AI.CanTranscribe() {
		t.Errorf("key, model and transcription model: on, got %+v", f.AI)
	}
	if f.AI.Model != "env-model" || f.AI.TimeoutSeconds != 60 {
		t.Errorf("the environment overrides the file field by field, got %+v", f.AI)
	}
	if f.AI.URL() != "https://llm.example.com/v1" {
		t.Errorf("trailing slash trimmed, got %q", f.AI.URL())
	}
	if !f.Feedback.AssistOn(f.AI) {
		t.Error("the assistant is on whenever the AI is")
	}

	t.Setenv("DDCORE_FEEDBACK_ASSIST", "0")
	if f, _, err = Load(site(t, `{}`)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Feedback.AssistOn(f.AI) {
		t.Error("DDCORE_FEEDBACK_ASSIST=0 turns the assistant off")
	}

	t.Setenv("DDCORE_AI", "0")
	if f, _, err = Load(site(t, `{}`)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.AI.On() {
		t.Error("DDCORE_AI=0 turns it off")
	}
}

func TestAIValidate(t *testing.T) {
	clearMailEnv(t)
	if _, _, err := Load(site(t, `{"ai":{"baseUrl":"ftp://x"}}`)); err == nil || !strings.Contains(err.Error(), "ai.baseUrl") {
		t.Errorf("a non-http address is refused, got %v", err)
	}
	t.Setenv("DDCORE_AI_TIMEOUT_SECONDS", "soon")
	if _, _, err := Load(site(t, `{}`)); err == nil || !strings.Contains(err.Error(), "DDCORE_AI_TIMEOUT_SECONDS") {
		t.Errorf("a timeout that is not a number is refused, got %v", err)
	}
}
