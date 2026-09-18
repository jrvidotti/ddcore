package engine

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
)

// mailApp adds two templates and a catalogue to the standard test app: an
// ordinary one, and one that declares its argument a credential.
func mailApp() map[string]string {
	return map[string]string{
		"mail/aviso.mail.ts": `import { defineMailTemplate, _ } from "@ddcore/sdk";
export default defineMailTemplate<{ pedido: string; nome: string; url: string }>({
  name: "demo.aviso",
  subject: (d) => _("Order {0} is ready", [d.pedido]),
  body: (d, b) => [
    b.p(_("Hello {0}.", [d.nome])),
    b.table([_("Item"), _("Qty")], [["Cadeira", "2"]]),
    b.button(_("Open"), d.url),
  ],
});`,
		"mail/segredo.mail.ts": `import { defineMailTemplate, _ } from "@ddcore/sdk";
export default defineMailTemplate<{ link: string }>({
  name: "demo.segredo",
  sensitive: true,
  subject: () => _("Your link"),
  body: (d, b) => [b.button(_("Open"), d.link)],
});`,
		"translations/pt-BR.csv": "Order {0} is ready,Pedido {0} está pronto,\nHello {0}.,Olá {0}.,\nOpen,Abrir,\nItem,Item,\nQty,Qtd,\n",
	}
}

func setupMail(t *testing.T) *Engine { return setupWith(t, mailApp()) }

func deliveries(t *testing.T, e *Engine) []map[string]any {
	t.Helper()
	rows, err := db.Select(context.Background(), e.DB.Pool,
		`SELECT name, "to", subject, status, template, lang, args, attachments, job, attempts, sent_at, error,
		        reference_doctype, reference_name
		 FROM tab_email_delivery ORDER BY creation`)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// The whole point of the record: a message that has not gone out yet is
// already accounted for, in the sender's own transaction.
func TestOPS02_SendingWritesARecordAndAJob(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SendTemplate("demo.aviso", "ana@x.com", map[string]any{
			"pedido": "PED-1", "nome": "Ana", "url": "https://example.com/1",
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	rows := deliveries(t, e)
	if len(rows) != 1 {
		t.Fatalf("expected one delivery record, got %d", len(rows))
	}
	r := rows[0]
	if got := db.Str(r["status"]); got != MailQueued {
		t.Errorf("status = %q, want %q", got, MailQueued)
	}
	// The test site's language is pt-BR and this recipient is not a user of it,
	// so the site's language is what the template renders in.
	if got := db.Str(r["subject"]); got != "Pedido PED-1 está pronto" {
		t.Errorf("subject = %q — the template did not render at queue time", got)
	}
	if got := db.Str(r["to"]); got != "ana@x.com" {
		t.Errorf("to = %q", got)
	}
	if args := asMap(r["args"]); db.Str(args["nome"]) != "Ana" {
		t.Errorf("arguments were not stored: %v", args)
	}

	jobs, err := db.Select(ctx, e.DB.Pool, `SELECT method, args FROM ddcore_job WHERE method = $1`, mailJobMethod)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected one queued job, got %d", len(jobs))
	}
	jobArgs := asMap(jobs[0]["args"])
	if db.Str(jobArgs["delivery"]) != db.Str(r["name"]) {
		t.Errorf("the job does not name the delivery: %v", jobArgs)
	}
	// An ordinary template's arguments live on the record, so the job payload
	// stays a pointer and nothing is duplicated into ddcore_job.
	if _, ok := jobArgs["args"]; ok {
		t.Errorf("an ordinary template must not copy its arguments into the job: %v", jobArgs)
	}
	if int64(toFloat(r["job"])) == 0 {
		t.Error("the record does not name its job")
	}
}

// Rollback must not send. Both writes are on the caller's transaction, so a
// request that fails leaves neither a job nor a record of a message nobody got.
func TestOPS02_RollbackSendsNothing(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		if err := c.SendTemplate("demo.aviso", "ana@x.com", map[string]any{"pedido": "P", "nome": "Ana", "url": "/x"}); err != nil {
			return err
		}
		return context.Canceled // anything at all, as long as it fails
	})
	if err == nil {
		t.Fatal("expected the transaction to fail")
	}

	if rows := deliveries(t, e); len(rows) != 0 {
		t.Errorf("a rolled back request left %d delivery record(s) behind", len(rows))
	}
	jobs, _ := db.Select(ctx, e.DB.Pool, `SELECT id FROM ddcore_job WHERE method = $1`, mailJobMethod)
	if len(jobs) != 0 {
		t.Errorf("a rolled back request queued %d job(s)", len(jobs))
	}
}

// A sensitive template's argument is a live credential. It may travel with the
// job, which is transient, and must never reach the record, which is not.
func TestOPS02_SensitiveTemplateStoresNoArguments(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	const link = "https://example.com/reset?token=deadbeef"
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SendTemplate("demo.segredo", "ana@x.com", map[string]any{"link": link})
	})
	if err != nil {
		t.Fatal(err)
	}

	rows := deliveries(t, e)
	if len(rows) != 1 {
		t.Fatalf("expected one delivery record, got %d", len(rows))
	}
	if args := asMap(rows[0]["args"]); len(args) != 0 {
		t.Errorf("a sensitive template stored its arguments: %v", args)
	}
	// Belt and braces: the token must not be anywhere in the row, including
	// the subject a careless template might have interpolated it into.
	for k, v := range rows[0] {
		if s := db.Str(v); strings.Contains(s, "deadbeef") {
			t.Errorf("the token reached tab_email_delivery in column %q: %s", k, s)
		}
	}

	jobs, _ := db.Select(ctx, e.DB.Pool, `SELECT args FROM ddcore_job WHERE method = $1`, mailJobMethod)
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	if inner := asMap(asMap(jobs[0]["args"])["args"]); db.Str(inner["link"]) != link {
		t.Errorf("a sensitive template's arguments did not reach the job: %v", inner)
	}
}

// The reader's language, not the sender's and not the site's. The sender here
// is Admin on a pt-BR site; the reader has chosen English.
func TestOPS02_MessageIsWrittenInTheRecipientsLanguage(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{"email": "ana@x.com", "full_name": "Ana", "language": "en"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		return c.SendTemplate("demo.aviso", "ana@x.com", map[string]any{"pedido": "PED-1", "nome": "Ana", "url": "/x"})
	})
	if err != nil {
		t.Fatal(err)
	}

	rows := deliveries(t, e)
	if len(rows) != 1 {
		t.Fatalf("expected one delivery record, got %d", len(rows))
	}
	if got := db.Str(rows[0]["lang"]); got != "en" {
		t.Errorf("lang = %q, want en", got)
	}
	if got := db.Str(rows[0]["subject"]); got != "Order PED-1 is ready" {
		t.Errorf("subject = %q — rendered in the sender's language, not the reader's", got)
	}
}

// An idempotency key is the caller saying "this is the same message". The
// unique index is what makes that hold under concurrency.
func TestOPS02_IdempotencyKeyRefusesASecondSend(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	send := func() error {
		return e.Run(ctx, "Admin", func(c *Ctx) error {
			_, err := c.QueueMail(MailRequest{
				Template: "demo.aviso", To: []string{"ana@x.com"}, Subject: "S", Lang: "en", Key: "pedido-1",
			})
			return err
		})
	}
	if err := send(); err != nil {
		t.Fatal(err)
	}
	if err := send(); err == nil {
		t.Fatal("the same key was accepted twice")
	}
	if rows := deliveries(t, e); len(rows) != 1 {
		t.Errorf("expected one record, got %d", len(rows))
	}

	// Without a key, two identical messages are two messages — resending an
	// invitation is a thing people do on purpose.
	for i := 0; i < 2; i++ {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			_, err := c.QueueMail(MailRequest{Template: "demo.aviso", To: []string{"ana@x.com"}, Subject: "S", Lang: "en"})
			return err
		})
		if err != nil {
			t.Fatalf("unkeyed send %d: %v", i, err)
		}
	}
	if rows := deliveries(t, e); len(rows) != 3 {
		t.Errorf("expected three records, got %d", len(rows))
	}
}

func TestOPS02_RefusesAnInvalidRecipient(t *testing.T) {
	e := setupMail(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		_, err := c.QueueMail(MailRequest{Template: "demo.aviso", To: []string{"not an address"}, Subject: "S"})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "not a valid email address") {
		t.Fatalf("expected an address refusal, got %v", err)
	}
}

func TestOPS02_RefusesAnUnknownTemplate(t *testing.T) {
	e := setupMail(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		return c.SendTemplate("demo.naoexiste", "ana@x.com", nil)
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected an unknown template to be refused in the caller's transaction, got %v", err)
	}
}

// mailUsers plants two ordinary accounts: one with the role that can read a
// Pessoa, one with nothing at all.
func mailUsers(t *testing.T, e *Engine) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, u := range []struct{ email, role string }{{"ana@x.com", "Gestor"}, {"ze@x.com", ""}} {
			d, err := c.NewDoc("User", Doc{"email": u.email, "full_name": u.email})
			if err != nil {
				return err
			}
			if u.role != "" {
				d["roles"] = []any{map[string]any{"role": u.role}}
			}
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		p, err := c.NewDoc("Pessoa", Doc{"nome": "Cliente"})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// file plants a File row owned by `owner`, optionally attached to a document.
func file(t *testing.T, e *Engine, owner, url string, size int, attachedTo string) string {
	t.Helper()
	var name string
	err := e.Run(context.Background(), owner, func(c *Ctx) error {
		values := Doc{"file_name": "nota.pdf", "file_url": url, "file_size": size, "content_type": "application/pdf"}
		if attachedTo != "" {
			values["attached_to_doctype"] = "Pessoa"
			values["attached_to_name"] = attachedTo
		}
		d, err := c.NewDoc("File", values)
		if err != nil {
			return err
		}
		saved, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		name = saved.Name()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func queueWithAttachment(e *Engine, user, attachment string) error {
	return e.Run(context.Background(), user, func(c *Ctx) error {
		_, err := c.QueueMail(MailRequest{
			Template: "demo.aviso", To: []string{"cliente@x.com"}, Subject: "S", Lang: "en",
			Attach: []string{attachment},
		})
		return err
	})
}

// "Authorized attachments" is one question asked once, in the sender's own
// context: read permission on the document a file hangs from is read permission
// on the file. A detached file belongs to its owner.
func TestOPS02_AttachmentNeedsPermission(t *testing.T) {
	e := setupMail(t)
	mailUsers(t, e)

	detached := file(t, e, "ze@x.com", "/private/files/a.pdf", 10, "")
	if err := queueWithAttachment(e, "ana@x.com", detached); err == nil {
		t.Error("a colleague's detached file was attached without permission")
	}
	if err := queueWithAttachment(e, "ze@x.com", detached); err != nil {
		t.Errorf("the owner may attach their own file: %v", err)
	}

	// Attached to a Pessoa, which a Gestor may read — so ana may send it.
	attached := file(t, e, "ze@x.com", "/private/files/b.pdf", 10, "Cliente")
	if err := queueWithAttachment(e, "ana@x.com", attached); err != nil {
		t.Errorf("read permission on the document should carry the file: %v", err)
	}
	// ze has no role at all, so the Pessoa is unreadable and so is its file.
	if err := queueWithAttachment(e, "ana@x.com", "no-such-file"); err == nil {
		t.Error("an attachment that does not exist was accepted")
	}
}

// The cap is checked where there is still someone to tell, not in a worker.
func TestOPS02_AttachmentsOverTheCapAreRefused(t *testing.T) {
	e := setupMail(t)
	mailUsers(t, e)
	e.Cfg.Mail.MaxAttachment = 100

	small := file(t, e, "ana@x.com", "/private/files/small.pdf", 40, "")
	if err := queueWithAttachment(e, "ana@x.com", small); err != nil {
		t.Fatalf("a file under the cap was refused: %v", err)
	}
	big := file(t, e, "ana@x.com", "/private/files/big.pdf", 400, "")
	err := queueWithAttachment(e, "ana@x.com", big)
	if err == nil || !strings.Contains(err.Error(), "bytes this site allows") {
		t.Fatalf("expected the cap to refuse the send, got %v", err)
	}
	if rows := deliveries(t, e); len(rows) != 1 {
		t.Errorf("a refused send left %d records, want 1", len(rows))
	}
}

// An attachment can also be named by the file_url an Attach field stores.
func TestOPS02_AttachmentResolvesByURL(t *testing.T) {
	e := setupMail(t)
	mailUsers(t, e)
	file(t, e, "ana@x.com", "/private/files/byurl.pdf", 10, "")
	if err := queueWithAttachment(e, "ana@x.com", "/private/files/byurl.pdf"); err != nil {
		t.Errorf("a file_url should resolve to its File: %v", err)
	}
}

// Deleting the order cannot un-send the invoice, so the record outlives the
// document it is about. Renaming, on the other hand, has to follow.
func TestOPS02_HistoryOutlivesItsDocument(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()
	mailUsers(t, e)

	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.QueueMail(MailRequest{
			Template: "demo.aviso", To: []string{"ana@x.com"}, Subject: "S", Lang: "en",
			Reference: &MailReference{Doctype: "Pessoa", Name: "Cliente"},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.Rename("Pessoa", "Cliente", "Cliente Novo")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows := deliveries(t, e)
	if len(rows) != 1 || db.Str(rows[0]["reference_name"]) != "Cliente Novo" {
		t.Fatalf("rename did not follow the reference: %v", rows)
	}

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.Delete("Pessoa", "Cliente Novo", true, false)
	}); err != nil {
		t.Fatal(err)
	}
	rows = deliveries(t, e)
	if len(rows) != 1 {
		t.Fatalf("deleting the document deleted the record of a message that already went out: %v", rows)
	}
	if got := db.Str(rows[0]["reference_name"]); got != "Cliente Novo" {
		t.Errorf("reference_name = %q — the record should still say what the message was about", got)
	}
}

// A save casts twice, and a JSON field used to come out of the second pass
// buried inside a JSON string. Version dodged it by writing its own SQL.
func TestOPS02_JSONFieldIsNotEncodedTwice(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SendTemplate("demo.aviso", "ana@x.com", map[string]any{"pedido": "PED-1", "nome": "Ana", "url": "/x"})
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT jsonb_typeof(args) AS kind, args->>'nome' AS nome FROM tab_email_delivery`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if got := db.Str(rows[0]["kind"]); got != "object" {
		t.Errorf("args is a JSON %s, want an object — the value was encoded twice", got)
	}
	if got := db.Str(rows[0]["nome"]); got != "Ana" {
		t.Errorf("args->>'nome' = %q, want Ana", got)
	}
}

// drainMail runs queued jobs until none are left.
//
// It polls rather than stopping at the first empty claim: run_after is written
// from the Go clock and claimed against Postgres's now(), so a job enqueued a
// microsecond ago is occasionally not yet visible to a worker asking for one.
func drainMail(t *testing.T, e *Engine) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ran, err := e.runOneJob(ctx)
		if err != nil {
			t.Fatalf("job: %v", err)
		}
		if ran {
			continue
		}
		rows, err := db.Select(ctx, e.DB.Pool, `SELECT count(*) AS n FROM ddcore_job WHERE status = 'queued'`)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) > 0 && int(toFloat(rows[0]["n"])) == 0 {
			failed, _ := db.Select(ctx, e.DB.Pool, `SELECT id, error FROM ddcore_job WHERE status = 'failed'`)
			for _, f := range failed {
				t.Errorf("job %v failed: %s", f["id"], db.Str(f["error"]))
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("jobs never drained")
}

// End to end on the log transport, which is the default and the one every
// developer meets first: queue a message, let a worker take it, and the record
// says it went.
func TestOPS02_WorkerDeliversAndRecordsTheOutcome(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	var log bytes.Buffer
	e.Log = slog.New(slog.NewTextHandler(&log, nil))
	e.mailOnce = sync.Once{} // rebuild the transport against the capturing log

	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SendTemplate("demo.aviso", "ana@x.com", map[string]any{
			"pedido": "PED-1", "nome": "Ana", "url": "https://example.com/1",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	drainMail(t, e)

	rows := deliveries(t, e)
	if len(rows) != 1 {
		t.Fatalf("expected one record, got %d", len(rows))
	}
	if got := db.Str(rows[0]["status"]); got != MailSent {
		t.Fatalf("status = %q, want %q", got, MailSent)
	}
	if n := int(toFloat(rows[0]["attempts"])); n != 1 {
		t.Errorf("attempts = %d, want 1", n)
	}

	out := log.String()
	// The body is rendered from the blocks, in the reader's language, with the
	// table laid out as columns and the button spelling its address out.
	for _, want := range []string{"Olá Ana.", "Item", "Cadeira", "Abrir: https://example.com/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in the delivered body:\n%s", want, out)
		}
	}
}

// A sensitive message renders from the job payload, because the record
// deliberately kept nothing to render from.
func TestOPS02_SensitiveMessageStillDelivers(t *testing.T) {
	e := setupMail(t)
	ctx := context.Background()

	var log bytes.Buffer
	e.Log = slog.New(slog.NewTextHandler(&log, nil))
	e.mailOnce = sync.Once{}

	const link = "https://example.com/reset?token=deadbeef"
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SendTemplate("demo.segredo", "ana@x.com", map[string]any{"link": link})
	})
	if err != nil {
		t.Fatal(err)
	}
	drainMail(t, e)

	rows := deliveries(t, e)
	if got := db.Str(rows[0]["status"]); got != MailSent {
		t.Fatalf("status = %q, want %q", got, MailSent)
	}
	if !strings.Contains(log.String(), link) {
		t.Errorf("the link never reached the message:\n%s", log.String())
	}
}
