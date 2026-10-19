package engine

import (
	"encoding/json"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/mail"
)

func init() {
	addHostOps(map[string]hostOp{
		// The language the message will be written in, decided before the
		// prelude renders anything: a reader gets their own language, not the
		// language of whoever pressed the button.
		"mail.prepare": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return map[string]any{"lang": c.RecipientLang(a.To)}, nil
		},
		"mail.queue": func(e *Engine, c *Ctx, _ *hostArgs, in hostReq) (any, error) {
			var r MailRequest
			if err := json.Unmarshal(in.raw, &r); err != nil {
				return nil, cerr.Internal("invalid arguments in {0}: {1}", in.op, err)
			}
			return c.QueueMail(r)
		},
		"mail.load": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return e.LoadMail(c, a.Delivery) },
		"mail.deliver": func(e *Engine, c *Ctx, a *hostArgs, in hostReq) (any, error) {
			var d struct {
				Blocks []mail.Block `json:"blocks"`
			}
			if err := json.Unmarshal(in.raw, &d); err != nil {
				return nil, cerr.Internal("invalid arguments in {0}: {1}", in.op, err)
			}
			return e.DeliverMail(c, a.Delivery, a.Subject, d.Blocks)
		},
		"mail.result": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return nil, e.RecordMail(c, a.Delivery, a.Status, a.Error)
		},
		"webhook.emit": func(e *Engine, c *Ctx, _ *hostArgs, in hostReq) (any, error) {
			var r struct {
				Event     string            `json:"event"`
				Data      any               `json:"data"`
				Key       string            `json:"key"`
				Reference *WebhookReference `json:"reference"`
			}
			if err := json.Unmarshal(in.raw, &r); err != nil {
				return nil, cerr.Internal("invalid arguments in {0}: {1}", in.op, err)
			}
			names, err := c.EmitWebhook(r.Event, r.Data, r.Reference, r.Key)
			return orErr(map[string]any{"deliveries": names}, err)
		},
		"webhook.validate": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return nil, c.ValidateWebhook(a.Doc) },
		"webhook.deliver": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return nil, e.DeliverWebhook(c, a.Delivery)
		},
		"webhook.replay": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			err := c.ReplayWebhook(a.Delivery)
			return orErr(map[string]any{"delivery": a.Delivery, "status": WebhookQueued}, err)
		},
		"webhook.sweep": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			n, err := e.SweepWebhookDeliveries(c.Ctx)
			return orErr(map[string]any{"deliveries": n}, err)
		},
		"push.send": func(e *Engine, c *Ctx, _ *hostArgs, in hostReq) (any, error) {
			var p pushSendArgs
			if err := json.Unmarshal(in.raw, &p); err != nil {
				return nil, cerr.Validation("push.send: {0}", err)
			}
			return e.PushSend(c.callCtx(), p)
		},
		// what a page subscribes with, unpadded as applicationServerKey takes
		// it; null until the site has a VAPID pair
		"push.publicKey": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			v, ok := e.Secret("vapid_public_key")
			return found(strings.TrimRight(strings.TrimSpace(v), "="), ok), nil
		},
	})
}
