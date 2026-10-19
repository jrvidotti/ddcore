package engine

import (
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

func init() {
	addHostOps(map[string]hostOp{
		// Key and Text are the secret and the message; the answer is hex unless
		// opts.output says otherwise, the form a webhook provider puts in its
		// signature header.
		"crypto.hmacSha256": func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return hmacSha256Op(a.Key, a.Text, a.Opts)
		},
		"crypto.sha256":       func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) { return sha256Op(a.Text, a.Opts) },
		"crypto.randomToken":  func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) { return randomTokenOp(a.Opts) },
		"crypto.randomInt":    func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) { return randomIntOp(a.Opts) },
		"crypto.randomString": func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) { return randomStringOp(a.Opts) },
		// Compared here, not in JS: `===` stops at the first differing byte,
		// which is what a signature check must not do.
		"crypto.timingSafeEqual": func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return subtle.ConstantTimeCompare([]byte(a.Key), []byte(a.Text)) == 1, nil
		},
		"crypto.pfxInfo": func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) {
			if a.ClientCert == nil {
				return nil, cerr.Validation("crypto: pfx is not valid base64")
			}
			return pfxInfo(a.ClientCert.Pfx, a.ClientCert.Password)
		},
		"crypto.certInfo": func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) {
			if a.ClientCert == nil {
				return nil, cerr.Validation("crypto: cert is not a valid PEM certificate")
			}
			return pemCertInfo(a.ClientCert.Cert)
		},
		"webhooks.verify": func(e *Engine, _ *Ctx, a *hostArgs, _ hostReq) (any, error) {
			// Never throws on a bad request: a receiver wants a boolean to refuse on.
			tol := 300.0
			if f, ok := a.Opts["toleranceSeconds"].(float64); ok {
				tol = f
			}
			return VerifyWebhook(a.Key, a.Headers, []byte(a.Text), tol, time.Now()), nil
		},
		"vault.set": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			valStr := ""
			if a.Value != nil {
				valStr = fmt.Sprint(a.Value)
			}
			if err := c.sharedVaultWrite(a.Opts); err != nil {
				return nil, err
			}
			return nil, e.VaultSet(c, a.Key, valStr)
		},
		"vault.get": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			get := e.VaultGet
			if a.Opts["shared"] == true {
				get = e.VaultGetShared
			}
			v, ok, err := get(c, a.Key)
			if err != nil || !ok {
				return nil, err
			}
			return map[string]any{"value": v}, nil
		},
		"vault.del": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			if err := c.sharedVaultWrite(a.Opts); err != nil {
				return nil, err
			}
			return nil, e.VaultDel(c, a.Key)
		},
		"vault.list": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return e.VaultList(c, a.Prefix) },
	})
}
