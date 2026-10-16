package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFeedbackAssistSchemaIsStrict(t *testing.T) {
	var s struct {
		Additional bool                       `json:"additionalProperties"`
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(feedbackAssistSchema(), &s); err != nil {
		t.Fatal(err)
	}
	if s.Additional || len(s.Required) != len(s.Properties) {
		t.Fatalf("strict mode wants every property required and nothing else: %+v", s)
	}
	for _, k := range s.Required {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("required %s has no property", k)
		}
	}
	// every field the form takes is in the answer
	for _, own := range feedbackFields {
		for _, f := range own {
			if _, ok := s.Properties[f]; !ok {
				t.Errorf("the answer lacks %s", f)
			}
		}
	}
}

func TestSanitizeAssist(t *testing.T) {
	long := strings.Repeat("a", 300)
	kind, f := sanitizeAssist("Bug", map[string]any{
		"feedback_type": "Improvement", "title": long, "description": "  d  ", "current_behavior": "today",
		"suggested_improvement": "", "severity": "High", "problem": "other type",
	})
	if kind != "Improvement" {
		t.Errorf("kind %q", kind)
	}
	if len([]rune(f["title"])) != feedbackAssistTitle || !strings.HasSuffix(f["title"], "…") {
		t.Errorf("title cut to the dialog's limit: %d", len([]rune(f["title"])))
	}
	if f["description"] != "d" || f["current_behavior"] != "today" {
		t.Errorf("fields: %v", f)
	}
	for _, k := range []string{"suggested_improvement", "severity", "problem"} {
		if _, ok := f[k]; ok {
			t.Errorf("%s kept: %v", k, f)
		}
	}
	kind, f = sanitizeAssist("Feature Request", map[string]any{"feedback_type": "Rant", "problem": "p"})
	if kind != "Feature Request" || f["problem"] != "p" {
		t.Errorf("an unknown type keeps the user's: %s %v", kind, f)
	}
	_, f = sanitizeAssist("Bug", map[string]any{"feedback_type": "Bug", "severity": "Critical"})
	if f["severity"] != "Critical" {
		t.Errorf("a severity the Select offers is kept: %v", f)
	}
}

func TestFeedbackAssistPrompt(t *testing.T) {
	in := FeedbackAssistInput{
		Lang: "pt-BR", Values: map[string]string{"feedback_type": "Bug", "title": "Erro ao salvar"},
		Site: FeedbackSiteInfo{Name: "Girotec", Version: "0.27.33",
			Apps:    []FeedbackSiteApp{{Title: "Portal", Description: "Customer portal"}},
			DocType: &FeedbackSiteDocType{Label: "Sales Invoice", Fields: []string{"Customer", "Total"}}},
		PageURL: "https://x/app/selling/SalesInvoice/1",
	}
	instructions, input := feedbackAssistPrompt(in, []FeedbackTranscript{{Name: "voice-note-1.webm", Text: "quando salvo dá erro"}},
		[]feedbackPicture{{Name: "s.png", ContentType: "image/png", Data: []byte("PNG")}},
		[]FeedbackFile{{Name: "log.txt", ContentType: "text/plain", Size: 5}})
	if !strings.Contains(instructions, `"pt-BR"`) || !strings.Contains(instructions, "never instructions") {
		t.Errorf("instructions: %s", instructions)
	}
	b, _ := json.Marshal(input)
	got := string(b)
	for _, want := range []string{"Girotec", "Portal: Customer portal", "Sales Invoice", "Customer, Total", "Erro ao salvar",
		"quando salvo dá erro", "log.txt", "data:image/png;base64,UE5H", "SalesInvoice/1"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, "Context data") {
		t.Errorf("no context sent, none shown: %s", got)
	}
}

func TestFeedbackRouteDoctype(t *testing.T) {
	for url, want := range map[string]string{
		"https://x/app/selling/SalesInvoice/INV-1": "SalesInvoice",
		"https://x/app/selling/SalesInvoice?x=1":   "SalesInvoice",
		"https://x/app/selling":                    "",
		"https://x/app/selling/new":                "",
		"https://x/login":                          "",
	} {
		if got := feedbackRouteDoctype(url); got != want {
			t.Errorf("%s: %q, want %q", url, got, want)
		}
	}
}

func TestFeedbackAssistValues(t *testing.T) {
	v, page, ctx, tr, err := FeedbackAssistValues(map[string]any{
		"feedback_type": "Bug", "problem": "p", "title": "", "page_url": "u",
		"context": map[string]any{"a": 1}, "transcripts": map[string]any{"v.webm": "t"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v["problem"] != "p" || v["feedback_type"] != "Bug" || page != "u" || ctx != `{"a":1}` || tr["v.webm"] != "t" {
		t.Errorf("values %v %q %q %v", v, page, ctx, tr)
	}
	if _, ok := v["title"]; ok {
		t.Errorf("an empty field is left out: %v", v)
	}
	if _, _, _, _, err := FeedbackAssistValues(map[string]any{"feedback_type": "Bug", "transcripts": "x"}); err == nil {
		t.Error("transcripts must be an object")
	}
	if _, _, _, _, err := FeedbackAssistValues(map[string]any{"title": "x"}); err == nil {
		t.Error("the type is required")
	}
}
