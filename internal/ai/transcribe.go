package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// MaxAudioBytes is the largest audio Transcribe sends: the provider's own limit.
const MaxAudioBytes = 25 << 20

// TranscribeRequest is one audio to turn into text.
type TranscribeRequest struct {
	// Audio is the bytes; from JS they come as Base64.
	Audio  []byte `json:"-"`
	Base64 string `json:"base64,omitempty"`
	// Filename is required: the provider infers the format from its extension.
	Filename    string `json:"filename"`
	ContentType string `json:"contentType,omitempty"`
	// Model is "" for the site's transcription model.
	Model string `json:"model,omitempty"`
	// Language is an optional ISO-639-1 hint, such as "pt".
	Language string `json:"language,omitempty"`
	// Prompt is optional text that steers spelling (names, jargon).
	Prompt string `json:"prompt,omitempty"`
	// Timeout is the whole call in seconds; 0 is DefaultTranscribeTimeout.
	Timeout float64 `json:"timeout,omitempty"`
}

// Transcription is the text of an audio.
type Transcription struct {
	Text  string `json:"text"`
	Model string `json:"model"`
}

// PrepareTranscription checks the request and decodes its audio, returning the
// address and the model the call would use. Pure, like Prepare.
func (c *Client) PrepareTranscription(req *TranscribeRequest) (url, model string, err error) {
	model = strings.TrimSpace(req.Model)
	if model == "" {
		model = c.cfg.TranscriptionModel
	}
	if model == "" {
		return "", "", cerr.Validation("No AI transcription model: set ai.transcriptionModel in ddcore.json or pass model")
	}
	if strings.TrimSpace(req.Filename) == "" {
		return "", "", cerr.Validation("ddcore.ai.transcribe needs a filename: the provider reads the format from its extension")
	}
	if req.Audio == nil && req.Base64 != "" {
		b, err := base64.StdEncoding.DecodeString(req.Base64)
		if err != nil {
			return "", "", cerr.Validation("ddcore.ai.transcribe: the audio is not valid base64")
		}
		req.Audio = b
	}
	if len(req.Audio) == 0 {
		return "", "", cerr.Validation("ddcore.ai.transcribe: the audio is empty")
	}
	if len(req.Audio) > MaxAudioBytes {
		return "", "", cerr.Validation("The audio is larger than {0} MB", MaxAudioBytes>>20)
	}
	return c.cfg.BaseURL + "/audio/transcriptions", model, nil
}

// Transcribe turns an audio into text.
func (c *Client) Transcribe(ctx context.Context, req TranscribeRequest) (*Transcription, error) {
	if c.cfg.APIKey == "" {
		return nil, notConfigured()
	}
	url, model, err := c.PrepareTranscription(&req)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", model)
	if req.Language != "" {
		_ = w.WriteField("language", req.Language)
	}
	if req.Prompt != "" {
		_ = w.WriteField("prompt", req.Prompt)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+escapeQuotes(req.Filename)+`"`)
	ct := req.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(req.Audio); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	raw, contentType := buf.Bytes(), w.FormDataContentType()
	status, resp, err := c.do(ctx, "transcribe", model, budget(req.Timeout, DefaultTranscribeTimeout), func(ctx context.Context) (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	return ParseTranscription(status, resp, model)
}

// ParseTranscription reads the audio endpoint's answer.
func ParseTranscription(status int, raw []byte, model string) (*Transcription, error) {
	if status != http.StatusOK {
		return nil, providerError(status, raw)
	}
	var data struct {
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Text == nil {
		return nil, cerr.Unavailable("AI request failed: {0}", "the transcription has no text")
	}
	return &Transcription{Text: strings.TrimSpace(*data.Text), Model: model}, nil
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string { return quoteEscaper.Replace(s) }
