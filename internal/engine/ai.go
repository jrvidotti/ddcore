package engine

import (
	"encoding/json"

	"github.com/jrvidotti/ddcore/internal/ai"
	"github.com/jrvidotti/ddcore/internal/cerr"
)

// AI is the client of the site's language-model provider. It is built per
// call from Cfg.AI, which costs a struct: connections are pooled by the
// shared transport, and a test that changes the configuration sees it.
func (e *Engine) AI() *ai.Client { return ai.New(ai.FromSite(e.Cfg.AI), e.Log) }

// aiHostCall serves the ddcore.ai.* ops. The prepare/parse ops exist for
// ddcore.test.fakeAi, which builds the body the real call would send and
// reads the fake's answer the way the real one is read; outside ddcore test
// they are refused.
func (e *Engine) aiHostCall(c *Ctx, op string, raw json.RawMessage) (any, error) {
	var q struct {
		Req    json.RawMessage `json:"req"`
		Status int             `json:"httpStatus"`
		Body   string          `json:"body"`
		JSON   bool            `json:"json"`
		Model  string          `json:"model"`
	}
	if err := json.Unmarshal(raw, &q); err != nil {
		return nil, cerr.Validation("Invalid arguments: {0}", err)
	}
	cl := e.AI()
	switch op {
	case "ai.enabled":
		return cl.Enabled(), nil
	case "ai.canTranscribe":
		return cl.CanTranscribe(), nil
	case "ai.respond", "ai.prepare":
		var req ai.Request
		if err := json.Unmarshal(q.Req, &req); err != nil {
			return nil, cerr.Validation("ddcore.ai.respond: {0}", err)
		}
		if op == "ai.respond" {
			return cl.Respond(c.callCtx(), req)
		}
		if err := e.requireTestMode(); err != nil {
			return nil, err
		}
		// a test runs with no key; the fake still sees the model default applied
		url, body, total, err := cl.Prepare(req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"url": url, "body": body, "timeout": total.Seconds()}, nil
	case "ai.transcribe", "ai.prepareTranscription":
		var req ai.TranscribeRequest
		if err := json.Unmarshal(q.Req, &req); err != nil {
			return nil, cerr.Validation("ddcore.ai.transcribe: {0}", err)
		}
		if op == "ai.transcribe" {
			return cl.Transcribe(c.callCtx(), req)
		}
		if err := e.requireTestMode(); err != nil {
			return nil, err
		}
		url, model, err := cl.PrepareTranscription(&req)
		if err != nil {
			return nil, err
		}
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = ai.DefaultTranscribeTimeout.Seconds()
		}
		return map[string]any{"url": url, "model": model, "timeout": timeout}, nil
	case "ai.parse", "ai.parseTranscription":
		if err := e.requireTestMode(); err != nil {
			return nil, err
		}
		if op == "ai.parse" {
			return ai.Parse(q.Status, []byte(q.Body), q.JSON)
		}
		return ai.ParseTranscription(q.Status, []byte(q.Body), q.Model)
	}
	return nil, cerr.Internal("unknown bridge operation: {0}", op)
}
