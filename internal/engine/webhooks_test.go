package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// receiver is a webhook endpoint that records every request and answers with
// whatever the test queues up, 200 once the queue runs out.
type receiver struct {
	*httptest.Server
	mu       sync.Mutex
	requests []receivedHook
	answers  []int
	delay    time.Duration
}

type receivedHook struct {
	ID, Timestamp, Signature string
	Body                     []byte
}

func newReceiver(t *testing.T, answers ...int) *receiver {
	r := &receiver{answers: answers}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.requests = append(r.requests, receivedHook{
			ID: req.Header.Get("webhook-id"), Timestamp: req.Header.Get("webhook-timestamp"),
			Signature: req.Header.Get("webhook-signature"), Body: body,
		})
		code := http.StatusOK
		if len(r.answers) > 0 {
			code, r.answers = r.answers[0], r.answers[1:]
		}
		delay := r.delay
		r.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(code)
		io.WriteString(w, "answer "+strconv.Itoa(code))
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *receiver) got() []receivedHook {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]receivedHook(nil), r.requests...)
}

// The key published with the Standard Webhooks test vector, which
// TestOPS06_SignatureMatchesTheStandardWebhooksVector checks us against. It is
// split in two because Stripe's webhook keys carry the same whsec_ prefix, and
// a secret scanner reading the whole literal reports the vector as a leak.
const hookSecret = "whsec_" + "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func setupWebhooks(t *testing.T) *Engine {
	t.Setenv("DDCORE_SECRET_KEY", "webhook-test-master-key")
	return setup(t)
}

// addWebhook subscribes url to Pessoa's insert and update unless values say
// otherwise.
func addWebhook(t *testing.T, e *Engine, url string, values Doc) string {
	t.Helper()
	v := Doc{"url": url, "event_type": "Document", "webhook_doctype": "Pessoa",
		"on_insert": true, "on_update": true, "secret": hookSecret, "max_attempts": 3}
	for k, x := range values {
		v[k] = x
	}
	var name string
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.NewDoc("Webhook", v)
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		name = saved.ID()
		return err
	})
	if err != nil {
		t.Fatalf("webhook: %v", err)
	}
	return name
}

func insertHookPessoa(t *testing.T, e *Engine, nome string) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		p, _ := c.NewDoc("Pessoa", Doc{"nome": nome, "segredo": "hunter2"})
		_, err := c.Insert(p, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func hookDeliveries(t *testing.T, e *Engine) []map[string]any {
	t.Helper()
	rows, err := db.Select(context.Background(), e.DB.Pool,
		`SELECT id, webhook, event, status, attempts, response_status, error, job, key,
		        reference_doctype, reference_id, payload::text AS payload
		 FROM tab_webhook_delivery ORDER BY creation`)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// runJobs runs every queued job, pulling a retry scheduled for later forward
// so a test does not wait out a real backoff.
func runJobs(t *testing.T, e *Engine) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET run_after = now() - interval '1 second' WHERE status = 'queued'`); err != nil {
			t.Fatal(err)
		}
		ran, err := e.runOneJob(ctx)
		if err != nil {
			t.Fatalf("job: %v", err)
		}
		if !ran {
			var n int
			e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM ddcore_job WHERE status IN ('queued', 'running')`).Scan(&n)
			if n == 0 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	t.Fatal("jobs never drained")
}

// The vector published with the Standard Webhooks reference libraries: if this
// drifts, every receiver verifying with one of them rejects us.
func TestOPS06_SignatureMatchesTheStandardWebhooksVector(t *testing.T) {
	got := SignWebhook(hookSecret, "msg_p5jXN8AQM9LWM0D4loKWxJek", 1614265330, []byte(`{"test": 2432232314}`))
	if got != "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=" {
		t.Fatalf("signature = %s", got)
	}
	if raw := SignWebhook("plain-secret", "id", 1, []byte("{}")); raw == got || !strings.HasPrefix(raw, "v1,") {
		t.Fatalf("a raw secret should sign with its own bytes: %s", raw)
	}
}

// The whole path: a document event becomes a record and a job on the writer's
// transaction, a worker posts it signed, and the body leaves every secret out.
func TestOPS06_DocumentEventIsDeliveredSigned(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	hook := addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Ana")

	ds := hookDeliveries(t, e)
	if len(ds) != 1 {
		t.Fatalf("deliveries = %v", ds)
	}
	d := ds[0]
	if db.Str(d["status"]) != WebhookQueued || db.Str(d["event"]) != "doc.on_insert" ||
		db.Str(d["webhook"]) != hook || db.Str(d["reference_id"]) != "Ana" || toFloat(d["job"]) == 0 {
		t.Fatalf("delivery = %v", d)
	}
	if strings.Contains(db.Str(d["payload"]), "hunter2") {
		t.Fatalf("a Password field reached the payload: %s", d["payload"])
	}
	var method, queue, backoff string
	e.DB.Pool.QueryRow(context.Background(), `SELECT method, queue, backoff FROM ddcore_job WHERE id = $1`,
		int64(toFloat(d["job"]))).Scan(&method, &queue, &backoff)
	if method != webhookJobMethod || queue != webhookQueue || backoff != BackoffExponential {
		t.Fatalf("job = %s %s %s", method, queue, backoff)
	}

	runJobs(t, e)
	got := rcv.got()
	if len(got) != 1 {
		t.Fatalf("receiver got %d requests", len(got))
	}
	r := got[0]
	ts, _ := strconv.ParseInt(r.Timestamp, 10, 64)
	if r.ID != db.Str(d["id"]) || r.Signature != SignWebhook(hookSecret, r.ID, ts, r.Body) {
		t.Fatalf("headers id=%s sig=%s", r.ID, r.Signature)
	}
	var env struct {
		Type string `json:"type"`
		Data struct {
			Doctype string         `json:"doctype"`
			ID      string         `json:"id"`
			Doc     map[string]any `json:"doc"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &env); err != nil {
		t.Fatal(err)
	}
	if env.Type != "doc.on_insert" || env.Data.Doctype != "Pessoa" || env.Data.ID != "Ana" || env.Data.Doc["nome"] != "Ana" {
		t.Fatalf("envelope = %s", r.Body)
	}
	if env.Data.Doc["segredo"] != nil {
		t.Fatalf("password in body: %s", r.Body)
	}
	after := hookDeliveries(t, e)[0]
	if db.Str(after["status"]) != WebhookSent || toFloat(after["attempts"]) != 1 || toFloat(after["response_status"]) != 200 {
		t.Fatalf("after delivery = %v", after)
	}
}

func TestOPS06_RollbackSendsNothing(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, nil)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Bia"})
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			return err
		}
		return cerr.Validation("changed my mind")
	})
	if err == nil {
		t.Fatal("expected the rollback")
	}
	if ds := hookDeliveries(t, e); len(ds) != 0 {
		t.Fatalf("deliveries after rollback = %v", ds)
	}
	var jobs int
	e.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM ddcore_job`).Scan(&jobs)
	if jobs != 0 {
		t.Fatalf("jobs after rollback = %d", jobs)
	}
	runJobs(t, e)
	if len(rcv.got()) != 0 {
		t.Fatal("the receiver heard about a write that never happened")
	}
}

// A receiver that fails and then recovers gets the same event twice under the
// same id — which is what lets it throw the second one away.
func TestOPS06_ReceiverFailureRetriesWithTheSameIdentity(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t, 500)
	addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Caio")
	ctx := context.Background()

	// run_after is written from the Go clock and claimed against Postgres's;
	// pull it back so the claim cannot race the clocks.
	e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET run_after = now() - interval '1 second'`)
	if ran, err := e.runOneJob(ctx); err != nil || !ran {
		t.Fatalf("first attempt ran=%v err=%v", ran, err)
	}
	d := hookDeliveries(t, e)[0]
	if db.Str(d["status"]) != WebhookRetrying || !strings.Contains(db.Str(d["error"]), "500") {
		t.Fatalf("after a 500 = %v", d)
	}
	// Exponential backoff: the first retry waits thirty seconds.
	var wait float64
	e.DB.Pool.QueryRow(ctx, `SELECT extract(epoch FROM run_after - now()) FROM ddcore_job WHERE id = $1`,
		int64(toFloat(d["job"]))).Scan(&wait)
	if wait < 20 || wait > 31 {
		t.Fatalf("first retry waits %.1fs", wait)
	}

	runJobs(t, e)
	got := rcv.got()
	if len(got) != 2 || got[0].ID != got[1].ID || string(got[0].Body) != string(got[1].Body) {
		t.Fatalf("requests = %+v", got)
	}
	d = hookDeliveries(t, e)[0]
	if db.Str(d["status"]) != WebhookSent || toFloat(d["attempts"]) != 2 || d["error"] != nil {
		t.Fatalf("after recovery = %v", d)
	}
}

func TestOPS06_ExponentialBackoffGrowsAndIsCapped(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var id int64
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.loop.ok", nil, map[string]any{"backoff": "exponential", "maxAttempts": 20})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		attempts int
		want     float64
	}{{1, 30}, {2, 60}, {3, 120}, {12, 3600}} {
		var got float64
		// run_after reads attempts as the row holds it, which is how the worker
		// calls it: the claim has already counted the attempt that just failed.
		if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET attempts = $2 WHERE id = $1`, id, c.attempts); err != nil {
			t.Fatal(err)
		}
		if err := e.DB.Pool.QueryRow(ctx, `UPDATE ddcore_job SET run_after = `+retryDelaySQL+`
			WHERE id = $1 RETURNING extract(epoch FROM run_after - now())`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got < c.want-1 || got > c.want+1 {
			t.Errorf("attempt %d waits %.0fs, want %.0f", c.attempts, got, c.want)
		}
	}
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'failed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	res, err := e.RetryJob(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	var backoff string
	e.DB.Pool.QueryRow(ctx, `SELECT backoff FROM ddcore_job WHERE id = $1`, res.NewID).Scan(&backoff)
	if backoff != BackoffExponential {
		t.Fatalf("a retried job lost its backoff: %q", backoff)
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.Enqueue("demo.services.loop.ok", nil, map[string]any{"backoff": "random"})
		return err
	})
	if err == nil {
		t.Fatal("an unknown backoff was accepted")
	}
}

func TestOPS06_ClientErrorIsNotRetried(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t, 400)
	addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Davi")
	runJobs(t, e)

	if n := len(rcv.got()); n != 1 {
		t.Fatalf("a 400 was tried %d times", n)
	}
	d := hookDeliveries(t, e)[0]
	if db.Str(d["status"]) != WebhookFailed || toFloat(d["response_status"]) != 400 ||
		!strings.Contains(db.Str(d["error"]), "answer 400") {
		t.Fatalf("delivery = %v", d)
	}
}

func TestOPS06_ExhaustedAttemptsFail(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t, 503, 503, 503)
	addWebhook(t, e, rcv.URL, Doc{"max_attempts": 2})
	insertHookPessoa(t, e, "Eva")
	runJobs(t, e)

	if n := len(rcv.got()); n != 2 {
		t.Fatalf("attempts = %d", n)
	}
	d := hookDeliveries(t, e)[0]
	if db.Str(d["status"]) != WebhookFailed || toFloat(d["attempts"]) != 2 {
		t.Fatalf("delivery = %v", d)
	}
	var status string
	e.DB.Pool.QueryRow(context.Background(), `SELECT status FROM ddcore_job WHERE id = $1`, int64(toFloat(d["job"]))).Scan(&status)
	if status != "failed" {
		t.Fatalf("job = %s", status)
	}
}

// A timeout is ambiguous — the receiver may have acted — and is retried anyway,
// under the same id.
func TestOPS06_TimeoutIsRetried(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	rcv.delay = 1500 * time.Millisecond
	addWebhook(t, e, rcv.URL, Doc{"timeout": 1, "max_attempts": 2})
	insertHookPessoa(t, e, "Fabi")
	runJobs(t, e)

	got := rcv.got()
	if len(got) != 2 || got[0].ID != got[1].ID {
		t.Fatalf("requests = %d", len(got))
	}
	d := hookDeliveries(t, e)[0]
	if db.Str(d["status"]) != WebhookFailed || !strings.Contains(db.Str(d["error"]), "Timeout") {
		t.Fatalf("delivery = %v", d)
	}
}

func TestOPS06_DisabledWebhookDoesNotSend(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	hook := addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Gil")
	if _, err := e.DB.Pool.Exec(context.Background(), `UPDATE tab_webhook SET enabled = false WHERE id = $1`, hook); err != nil {
		t.Fatal(err)
	}
	runJobs(t, e)
	if len(rcv.got()) != 0 {
		t.Fatal("a disabled webhook sent")
	}
	if d := hookDeliveries(t, e)[0]; db.Str(d["status"]) != WebhookFailed || !strings.Contains(db.Str(d["error"]), "disabled") {
		t.Fatalf("delivery = %v", d)
	}
}

// Submitting and deleting are events of their own, and the record of a delete
// outlives the document it describes.
func TestOPS06_SubmitAndDeleteAreEvents(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, Doc{"webhook_doctype": "Pedido", "on_insert": false, "on_update": false, "on_submit": true, "on_cancel": true})
	addWebhook(t, e, rcv.URL, Doc{"on_insert": false, "on_update": false, "on_trash": true})
	insertHookPessoa(t, e, "Hugo")
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		p, _ := c.NewDoc("Pedido", Doc{"cliente": "Hugo"})
		p, err := c.Insert(p, SaveOpts{})
		if err != nil {
			return err
		}
		p, err = c.Submit(p)
		if err != nil {
			return err
		}
		if _, err := c.Cancel(p); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error { return c.Delete("Pessoa", "Hugo", false, true) }); err != nil {
		t.Fatal(err)
	}
	var events []string
	for _, d := range hookDeliveries(t, e) {
		events = append(events, db.Str(d["event"])+":"+db.Str(d["reference_doctype"]))
	}
	if strings.Join(events, ",") != "doc.on_submit:Pedido,doc.on_cancel:Pedido,doc.on_trash:Pessoa" {
		t.Fatalf("events = %v", events)
	}
}

func TestOPS06_ReplayResendsTheSameEventAndAudits(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t, 400)
	addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Iris")
	runJobs(t, e)
	d := hookDeliveries(t, e)[0]
	name := db.Str(d["id"])
	ctx := context.Background()

	// Somebody without the role is refused, and the refusal is on record even
	// though its transaction rolled back.
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "joao@x.com", "full_name": "João"})
		_, err := c.Insert(u, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "joao@x.com", func(c *Ctx) error { return c.ReplayWebhook(name) })
	if cerr.From(err).Type != cerr.From(cerr.Permission("x")).Type {
		t.Fatalf("replay without the role = %v", err)
	}

	if err := e.Run(ctx, "Admin", func(c *Ctx) error { return c.ReplayWebhook(name) }); err != nil {
		t.Fatal(err)
	}
	// A second replay while the first is still queued would send it twice.
	err = e.Run(ctx, "Admin", func(c *Ctx) error { return c.ReplayWebhook(name) })
	if err == nil || !strings.Contains(err.Error(), "still on its way") {
		t.Fatalf("replay of a queued delivery = %v", err)
	}

	runJobs(t, e)
	got := rcv.got()
	if len(got) != 2 || got[0].ID != got[1].ID || string(got[0].Body) != string(got[1].Body) {
		t.Fatalf("requests = %+v", got)
	}
	if after := hookDeliveries(t, e)[0]; db.Str(after["status"]) != WebhookSent || toFloat(after["attempts"]) != 1 {
		t.Fatalf("after replay = %v", after)
	}

	rows, err := db.Select(ctx, e.DB.Pool, `SELECT action, outcome, actor, target_doctype, target_id, detail::text AS detail
		FROM tab_audit_event WHERE action = 'webhook.replay' ORDER BY creation`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("audit = %v", rows)
	}
	if db.Str(rows[0]["outcome"]) != "Denied" || db.Str(rows[0]["actor"]) != "joao@x.com" {
		t.Fatalf("denied row = %v", rows[0])
	}
	if db.Str(rows[1]["outcome"]) != "Allowed" || db.Str(rows[1]["action"]) != "webhook.replay" ||
		db.Str(rows[1]["target_id"]) != name || !strings.Contains(db.Str(rows[1]["detail"]), "Failed") {
		t.Fatalf("allowed row = %v", rows[1])
	}
	if strings.Contains(db.Str(rows[1]["detail"]), hookSecret) {
		t.Fatal("the audit detail carries the secret")
	}
}

func TestOPS06_EmitCustomEvent(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, Doc{"event_type": "Custom", "custom_event": "shop.order_paid", "webhook_doctype": ""})
	ctx := context.Background()

	emit := func(key string) ([]string, error) {
		var out []string
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			rt, err := c.RT()
			if err != nil {
				return err
			}
			v, err := rt.Eval(`ddcore.webhooks.emit("shop.order_paid", { order: "SO-1", total: 10 }, { key: "` + key + `" })`)
			if err != nil {
				return err
			}
			var r struct {
				Deliveries []string `json:"deliveries"`
			}
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			out = r.Deliveries
			return nil
		})
		return out, err
	}
	names, err := emit("so-1")
	if err != nil || len(names) != 1 {
		t.Fatalf("emit = %v %v", names, err)
	}
	if _, err := emit("so-1"); err == nil {
		t.Fatal("the same key was emitted twice")
	}
	runJobs(t, e)
	got := rcv.got()
	if len(got) != 1 {
		t.Fatalf("requests = %d", len(got))
	}
	var env struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(got[0].Body, &env); err != nil || env.Type != "shop.order_paid" || env.Data["order"] != "SO-1" {
		t.Fatalf("body = %s (%v)", got[0].Body, err)
	}

	for _, bad := range []string{"doc.on_submit", "Shop Paid", ""} {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			_, err := c.EmitWebhook(bad, nil, nil, "")
			return err
		})
		if err == nil {
			t.Errorf("event %q was accepted", bad)
		}
	}
}

// A worker that dies mid-delivery leaves a running job with a lease; the sweep
// gives it back and the next worker delivers it.
func TestOPS06_CrashedWorkerResumes(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Jade")
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'running', attempts = 1,
		started = now(), lease_until = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	runJobs(t, e)
	if len(rcv.got()) != 1 {
		t.Fatalf("requests = %d", len(rcv.got()))
	}
	if d := hookDeliveries(t, e)[0]; db.Str(d["status"]) != WebhookSent {
		t.Fatalf("delivery = %v", d)
	}
}

func TestOPS06_WebhooksOffQueuesNothing(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, nil)
	e.Cfg.Webhooks.Off = true
	insertHookPessoa(t, e, "Kiko")
	if ds := hookDeliveries(t, e); len(ds) != 0 {
		t.Fatalf("deliveries with webhooks off = %v", ds)
	}
}

func TestOPS06_WebhookValidation(t *testing.T) {
	e := setupWebhooks(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		values Doc
		want   string
	}{
		{"internal doctype", Doc{"webhook_doctype": "Webhook Delivery"}, "cannot be watched"},
		{"child table", Doc{"webhook_doctype": "Item Pedido"}, "cannot be watched"},
		{"unknown doctype", Doc{"webhook_doctype": "Nada"}, "does not exist"},
		{"plain http", Doc{"url": "http://example.com/hook"}, "https"},
		{"not a url", Doc{"url": "ftp://example.com"}, "http or https"},
		{"no event", Doc{"on_insert": false, "on_update": false}, "at least one"},
		{"forged event", Doc{"event_type": "Custom", "custom_event": "doc.on_insert"}, "not a valid"},
		{"timeout", Doc{"timeout": 600}, "Timeout"},
	}
	for _, tc := range cases {
		v := Doc{"url": "https://example.com/hook", "event_type": "Document", "webhook_doctype": "Pessoa",
			"on_insert": true, "secret": hookSecret, "timeout": 10, "max_attempts": 3}
		for k, x := range tc.values {
			v[k] = x
		}
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, _ := c.NewDoc("Webhook", v)
			_, err := c.Insert(doc, SaveOpts{})
			return err
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
	// Plain http is fine for a receiver on this machine.
	addWebhook(t, e, "http://127.0.0.1:9/hook", nil)
}

func TestOPS06_SweepKeepsDeliveriesOnTheirWay(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, nil)
	insertHookPessoa(t, e, "Leo")
	insertHookPessoa(t, e, "Lia")
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE tab_webhook_delivery SET modified = now() - interval '90 days',
		status = CASE WHEN reference_id = 'Leo' THEN 'Sent' ELSE 'Retrying' END`); err != nil {
		t.Fatal(err)
	}
	n, err := e.SweepWebhookDeliveries(ctx)
	if err != nil || n != 1 {
		t.Fatalf("swept %d, %v", n, err)
	}
	if ds := hookDeliveries(t, e); len(ds) != 1 || db.Str(ds[0]["reference_id"]) != "Lia" {
		t.Fatalf("left = %v", ds)
	}
}

// A user with access scopes cannot administer webhooks: a subscription would
// send every document of a DocType to an outside address, and a delivery's
// payload is not filtered by scope. A document write of the same user still
// queues its deliveries.
func TestOPS06_ScopedUsersCannotAdministerWebhooks(t *testing.T) {
	e := setupWebhooks(t)
	rcv := newReceiver(t)
	ctx := context.Background()
	const scoped, unscoped = "scoped@x.com", "unscoped@x.com"
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, user := range []string{scoped, unscoped} {
			u, err := c.NewDoc("User", Doc{"email": user, "full_name": user, "roles": []any{
				map[string]any{"role": "System Manager"}, map[string]any{"role": "Gestor"},
			}})
			if err != nil {
				return err
			}
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		_, err := c.Insert(Doc{"doctype": "User Permission", "user": scoped, "allow": "Pessoa", "for_value": "Iris"}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	addWebhook(t, e, rcv.URL, nil)

	// The scoped user's in-scope write succeeds and queues its delivery.
	err = e.Run(ctx, scoped, func(c *Ctx) error {
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Iris"})
		_, err := c.Insert(p, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatalf("scoped insert: %v", err)
	}
	deliveries := hookDeliveries(t, e)
	if len(deliveries) != 1 || db.Str(deliveries[0]["reference_id"]) != "Iris" {
		t.Fatalf("deliveries = %v", deliveries)
	}
	runJobs(t, e)
	delivery := db.Str(deliveries[0]["id"])
	if got := hookDeliveries(t, e)[0]; db.Str(got["status"]) != WebhookSent || len(rcv.got()) != 1 {
		t.Fatalf("scoped user's delivery = %v, requests = %d", got, len(rcv.got()))
	}
	hook := db.Str(deliveries[0]["webhook"])

	isPermission := func(err error) bool {
		return err != nil && cerr.From(err).Type == cerr.From(cerr.Permission("x")).Type
	}
	newHook := func(c *Ctx) error {
		doc, err := c.NewDoc("Webhook", Doc{"url": rcv.URL, "event_type": "Document", "webhook_doctype": "Pessoa",
			"on_insert": true, "secret": hookSecret, "max_attempts": 3})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, SaveOpts{})
		return err
	}
	listDeliveries := func(c *Ctx) error {
		_, err := c.GetList("Webhook Delivery", ListArgs{Fields: []string{"id", "payload"}})
		return err
	}
	replay := func(c *Ctx) error { return c.ReplayWebhook(delivery) }

	for i, fn := range []func(*Ctx) error{newHook, listDeliveries, replay} {
		if err := e.Run(ctx, scoped, fn); !isPermission(err) {
			t.Fatalf("scoped action %d = %v", i, err)
		}
	}
	var denied int
	if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM tab_audit_event
		WHERE action = 'webhook.replay' AND outcome = 'Denied' AND actor = $1`, scoped).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 1 {
		t.Fatalf("denied replay audit rows = %d", denied)
	}

	// Skipping role permissions does not skip the scope: app code calling
	// getAll, getValue, exists, insert or dbSet with ignorePermissions is
	// refused the same way.
	err = e.Run(ctx, scoped, func(c *Ctx) error {
		for _, dt := range []string{"Webhook", "Webhook Delivery"} {
			rows, err := c.GetList(dt, ListArgs{Fields: []string{"id"}, IgnorePermissions: true})
			if err != nil {
				return err
			}
			if len(rows) != 0 {
				t.Errorf("scoped getAll %s = %v", dt, rows)
			}
		}
		if ok, err := c.Exists("Webhook Delivery", delivery); err != nil || ok {
			t.Errorf("scoped exists = %v, %v", ok, err)
		}
		if v, err := c.GetValue("Webhook", hook, "url"); err != nil || (v != nil && v != "") {
			t.Errorf("scoped getValue = %v, %v", v, err)
		}
		doc, err := c.NewDoc("Webhook", Doc{"url": rcv.URL, "event_type": "Document", "webhook_doctype": "Pessoa",
			"on_insert": true, "secret": hookSecret, "max_attempts": 3})
		if err != nil {
			return err
		}
		if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); !isPermission(err) {
			t.Errorf("scoped insert ignoring permissions = %v", err)
		}
		if _, err := c.DBSet("Webhook", hook, Doc{"url": "https://attacker.example"}, true); !isPermission(err) {
			t.Errorf("scoped dbSet = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, fn := range []func(*Ctx) error{newHook, listDeliveries, replay} {
		if err := e.Run(ctx, unscoped, fn); err != nil {
			t.Fatalf("unscoped action %d = %v", i, err)
		}
	}
}

// A typed error raised in Go and let through by a service must reach the HTTP
// border with its type. It used to arrive as a 500 ScriptError, because goja
// appends the stack after the error's JSON and the decoder refused the tail.
func TestOPS06_ReplayErrorKeepsItsTypeThroughTheBridge(t *testing.T) {
	e := setupWebhooks(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		_, err = rt.CallFunction("core.services.webhooks.replay", json.RawMessage(`{"delivery":"nope"}`))
		return err
	})
	ce := cerr.From(err)
	if ce.Type != "DoesNotExistError" || ce.Status != 404 {
		t.Fatalf("error = %s %d: %v", ce.Type, ce.Status, err)
	}
}
