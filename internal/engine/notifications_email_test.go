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

// The framework's own notifications (assignment, share, task due) also go out
// by email, unless the recipient turned that kind off on their profile.

func shareN1(t *testing.T, e *Engine, to string) {
	t.Helper()
	runAs(t, e, shareSM, func(c *Ctx) error {
		_, err := c.ShareDoc("Shared Note", "N1", to, ShareRights{})
		return err
	})
}

func inboxCount(t *testing.T, e *Engine, user string) int {
	t.Helper()
	rows, err := db.Select(context.Background(), e.DB.Pool,
		`SELECT count(*) AS n FROM ddcore_notification WHERE recipient=$1 AND desk`, user)
	if err != nil {
		t.Fatal(err)
	}
	return int(toFloat(rows[0]["n"]))
}

func setUserColumn(t *testing.T, e *Engine, user, assignment string) {
	t.Helper()
	if _, err := e.DB.Pool.Exec(context.Background(), `UPDATE tab_user SET `+assignment+` WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
}

func TestCoreMail_ShareQueuesMailInRecipientLanguage(t *testing.T) {
	e := setupShare(t)
	e.Cfg.SiteURL = "https://erp.example.com/"
	setUserColumn(t, e, sharePlain, "language='pt-BR'")
	shareN1(t, e, sharePlain)

	rows := deliveries(t, e)
	if len(rows) != 1 {
		t.Fatalf("expected one delivery, got %+v", rows)
	}
	r := rows[0]
	if db.Str(r["template"]) != MailTemplateNotification || db.Str(r["lang"]) != "pt-BR" ||
		!strings.Contains(db.Str(r["to"]), sharePlain) {
		t.Fatalf("delivery = %+v", r)
	}
	if got := db.Str(r["subject"]); !strings.HasPrefix(got, "Compartilhado com você: ") {
		t.Fatalf("subject = %q", got)
	}
	if db.Str(r["reference_doctype"]) != "Shared Note" || db.Str(r["reference_id"]) != "N1" {
		t.Fatalf("reference = %+v", r)
	}
	// the delivery is tied to the inbox row, so access is checked again at send time
	linked, err := db.Select(context.Background(), e.DB.Pool,
		`SELECT email_delivery FROM ddcore_notification WHERE recipient=$1`, sharePlain)
	if err != nil || len(linked) != 1 || db.Str(linked[0]["email_delivery"]) != db.Str(r["id"]) {
		t.Fatalf("notification link = %+v, %v", linked, err)
	}

	// the log transport prints the message it would have sent
	var log bytes.Buffer
	e.Log = slog.New(slog.NewTextHandler(&log, nil))
	e.mailOnce = sync.Once{}
	drainMail(t, e)
	if got := deliveries(t, e)[0]["status"]; got != MailSent {
		t.Fatalf("status = %v", got)
	}
	if !strings.Contains(log.String(), "https://erp.example.com/app/Shared%20Note/N1") {
		t.Fatalf("no document link in the message:\n%s", log.String())
	}
}

func TestCoreMail_MutedRecipientGetsInboxOnly(t *testing.T) {
	e := setupShare(t)
	setUserColumn(t, e, sharePlain, "mute_share_email=true")
	shareN1(t, e, sharePlain)
	if n := inboxCount(t, e, sharePlain); n != 1 {
		t.Fatalf("inbox = %d, want 1", n)
	}
	if rows := deliveries(t, e); len(rows) != 0 {
		t.Fatalf("a muted recipient was mailed: %+v", rows)
	}
}

// Another kind's switch does not mute this one, and a column that predates the
// field (NULL) still sends.
func TestCoreMail_PreferenceIsPerKindAndNullSends(t *testing.T) {
	e := setupShare(t)
	setUserColumn(t, e, sharePlain, "mute_share_email=NULL, mute_assignment_email=true, mute_due_email=true")
	shareN1(t, e, sharePlain)
	if rows := deliveries(t, e); len(rows) != 1 {
		t.Fatalf("expected one delivery, got %+v", rows)
	}

	runAs(t, e, shareSM, func(c *Ctx) error {
		return c.NotifyUser(sharePlain, "Shared Note", "N1", "Assigned: Shared Note N1", "do it")
	})
	if n := inboxCount(t, e, sharePlain); n != 2 {
		t.Fatalf("inbox = %d, want 2", n)
	}
	if rows := deliveries(t, e); len(rows) != 1 {
		t.Fatalf("assignment mail went to a recipient who muted it: %+v", rows)
	}
}

func TestCoreMail_AssignmentQueuesMail(t *testing.T) {
	e := setupShare(t)
	runAs(t, e, shareSM, func(c *Ctx) error {
		return c.NotifyUser(shareEditor, "Shared Note", "N1", "Assigned: Shared Note N1", "line one\nline two")
	})
	rows := deliveries(t, e)
	if len(rows) != 1 || db.Str(rows[0]["subject"]) != "Assigned: Shared Note N1" {
		t.Fatalf("deliveries = %+v", rows)
	}
	drainMail(t, e)
}

// Admin's address is the literal "Admin": nothing to mail, nothing refused.
func TestCoreMail_UnmailableAddressSkips(t *testing.T) {
	e := setupShare(t)
	runAs(t, e, shareSM, func(c *Ctx) error {
		return c.NotifyUser("Admin", "Shared Note", "N1", "Assigned: Shared Note N1", "x")
	})
	if n := inboxCount(t, e, "Admin"); n != 1 {
		t.Fatalf("inbox = %d, want 1", n)
	}
	if rows := deliveries(t, e); len(rows) != 0 {
		t.Fatalf("deliveries = %+v", rows)
	}
}

func TestCoreMail_SelfActionSendsNothing(t *testing.T) {
	e := setupShare(t)
	runAs(t, e, shareSM, func(c *Ctx) error {
		return c.NotifyUser(shareSM, "Shared Note", "N1", "Assigned: Shared Note N1", "x")
	})
	if n, rows := inboxCount(t, e, shareSM), deliveries(t, e); n != 0 || len(rows) != 0 {
		t.Fatalf("inbox = %d, deliveries = %+v", n, rows)
	}
}

// The email is a courtesy copy: when it cannot be queued, the inbox row stays.
func TestCoreMail_QueueFailureKeepsInbox(t *testing.T) {
	e := setupShare(t)
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `CREATE FUNCTION refuse_delivery() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'no deliveries today'; END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Pool.Exec(ctx, `CREATE TRIGGER refuse_delivery BEFORE INSERT ON tab_email_delivery
		FOR EACH ROW EXECUTE FUNCTION refuse_delivery()`); err != nil {
		t.Fatal(err)
	}
	shareN1(t, e, sharePlain)
	if n := inboxCount(t, e, sharePlain); n != 1 {
		t.Fatalf("inbox = %d, want 1", n)
	}
	if rows := deliveries(t, e); len(rows) != 0 {
		t.Fatalf("deliveries = %+v", rows)
	}
}

func TestCoreMail_AccessRevokedBeforeSend(t *testing.T) {
	e := setupShare(t)
	ctx := context.Background()
	shareN1(t, e, sharePlain)
	runAs(t, e, shareSM, func(c *Ctx) error { return c.UnshareDoc("Shared Note", "N1", sharePlain) })
	for _, row := range deliveries(t, e) {
		if _, err := e.RunJob(ctx, "Admin", mailJobMethod, map[string]any{"delivery": row["id"]}); err != nil {
			t.Fatal(err)
		}
	}
	rows := deliveries(t, e)
	if len(rows) != 1 || rows[0]["status"] != MailFailed {
		t.Fatalf("revoked mail was sent: %+v", rows)
	}
}

func insertDueToDo(t *testing.T, e *Engine, to, description string) {
	t.Helper()
	runAs(t, e, "Admin", func(c *Ctx) error {
		return insertDoc(c, "ToDo", Doc{"allocated_to": to, "status": "Open",
			"date": time.Now().Format("2006-01-02"), "description": description})
	})
}

func TestCoreMail_DueReminder(t *testing.T) {
	e := setupShare(t)
	ctx := context.Background()
	setUserColumn(t, e, shareReader, "mute_due_email=true")
	// a multi-line description must not reach the subject as a line break, and
	// a ToDo allocated to Admin (no mailable address) must not stall the sweep
	insertDueToDo(t, e, "Admin", "Admin task")
	insertDueToDo(t, e, shareEditor, "Call the bank\nBcc: eve@x.com")
	insertDueToDo(t, e, shareReader, "Muted task")

	for i := 0; i < 2; i++ { // the second sweep must not mail again
		if err := e.SweepNotifications(ctx, time.Now()); err != nil {
			t.Fatalf("sweep %d: %v", i, err)
		}
	}
	for _, user := range []string{"Admin", shareEditor, shareReader} {
		if n := inboxCount(t, e, user); n != 1 {
			t.Fatalf("inbox of %s = %d, want 1", user, n)
		}
	}
	rows := deliveries(t, e)
	if len(rows) != 1 || !strings.Contains(db.Str(rows[0]["to"]), shareEditor) {
		t.Fatalf("deliveries = %+v", rows)
	}
	if got := db.Str(rows[0]["subject"]); got != "Assignment due today: Call the bank Bcc: eve@x.com" {
		t.Fatalf("subject = %q", got)
	}
	drainMail(t, e)
}

// An app's rule is not the recipient's to switch off, whatever it is called.
func TestCoreMail_AppRuleIgnoresMutePreference(t *testing.T) {
	files := notificationFixture()
	files["notifications/due.notification.ts"] = strings.Replace(files["notifications/due.notification.ts"],
		`name:"due"`, `name:"share"`, 1)
	e := setupWith(t, files)
	seedNotificationUsers(t, e)
	setUserColumn(t, e, "reader@example.com", "mute_share_email=true, mute_assignment_email=true, mute_due_email=true")
	insertReminder(t, e, "Historical", "2020-01-01", true)
	if err := e.SweepNotifications(context.Background(), time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	rows := deliveries(t, e)
	if len(rows) != 1 || db.Str(rows[0]["template"]) != "reminder" {
		t.Fatalf("deliveries = %+v", rows)
	}
}
