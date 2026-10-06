package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jrvidotti/ddcore/internal/js"
)

// errorLogTimeout bounds the write of one Error Log row. It runs on an error
// path, often on a context that is already cancelled, and must not hold a
// connection — or the code that asked for it — for longer than this.
const errorLogTimeout = 5 * time.Second

// Caps on what one app-recorded row may hold. The text column has no limit,
// and the row is written on the path where something already went wrong: a
// message that is a whole response body, or a context that is a whole
// document list, must not turn one failure into a megabyte per row.
const (
	errorLogMethodMax  = 200
	errorLogHeadMax    = 8 << 10
	errorLogStackMax   = 16 << 10
	errorLogContextMax = 16 << 10
)

// recordError writes an Error Log row on a transaction of its own and returns
// its id, so the row stays whatever becomes of the work that asked for it. The
// request id is the one ctx carries and the tenant the one ctx names, if any.
// The line goes to the process log first: if the row cannot be written, the
// line is what is left.
//
// vm, when not nil, is the VM of the code asking — the caller of
// ddcore.errorLog.record, which holds it while it waits. The row's own write
// runs its hooks on that VM instead of taking a second one from the pool: a
// pool every caller already holds a slot of would otherwise never hand one
// out. st is the state that VM was built for.
func (e *Engine) recordError(ctx context.Context, method, text string, st *State, vm *js.Runtime) (string, error) {
	id := RequestIDFrom(ctx)
	// The id is omitted rather than logged empty when no request is behind the
	// work: `id=""` on every migration and CLI error is noise that makes the
	// lines that do correlate harder to spot.
	if id != "" {
		e.Log.Error(text, "method", method, "id", id)
	} else {
		e.Log.Error(text, "method", method)
	}
	// The context of a failed request is very often already cancelled — the
	// client hung up, or a timeout is what failed it in the first place — and
	// the row explaining why is then the one thing that gets lost. Detach, but
	// keep a bound.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), errorLogTimeout)
	defer cancel()
	c := e.NewCtx(ctx, "Admin")
	if vm != nil {
		prev := vm.Ctx
		c.St, c.rt, c.rtLent = st, vm, true
		vm.Ctx = c
		defer func() { vm.Ctx = prev }()
	}
	var name string
	err := c.Run(func(c *Ctx) error {
		doc, err := c.NewDoc("Error Log", Doc{"method": method, "error": text, "request_id": id})
		if err != nil {
			return err
		}
		doc, err = c.Insert(doc, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		name = doc.ID()
		return nil
	})
	if err != nil {
		e.Log.Warn("Error Log row not written", "method", method, "err", err)
		return "", err
	}
	return name, nil
}

// AppError is what ddcore.errorLog.record was given, taken apart by the
// prelude: the parts of the error it caught and the caller's options.
type AppError struct {
	Method  string `json:"method"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	// Context is the caller's context, already JSON: the prelude serialises
	// it, so a value JSON cannot hold is dealt with where it was made.
	Context string `json:"context"`
}

// RecordAppError is ddcore.errorLog.record: an Error Log row the app files on
// purpose, on a transaction of its own, while its own work goes on — or rolls
// back without taking the row with it. It lands in c's tenant and carries the
// handle of the work c belongs to: the request's id, or `job:<id>` in a job's
// body, which has no request id of its own. vm is the VM making the call.
//
// It returns the row's id, or "" when the row could not be written: it is
// called from a catch block, and a throw there would fail the very work the
// caller is trying to keep. The process log has the line either way.
func (c *Ctx) RecordAppError(vm *js.Runtime, a AppError) string {
	reqID, method := c.ReqID, "app.record"
	if job := c.currentJob(); job != nil {
		if reqID == "" {
			reqID = fmt.Sprintf("job:%v", job["id"])
		}
		method = fmt.Sprintf("job:%v", job["method"])
	}
	if m := strings.TrimSpace(a.Method); m != "" {
		method = m
	}
	ctx := WithRequestID(WithTenant(c.Ctx, c.Tenant), reqID)
	name, _ := c.E.recordError(ctx, clip(method, errorLogMethodMax), a.text(), c.St, vm)
	return name
}

// text is the row's error column: `Type: title — message`, leaving out the
// parts there are not, then the stack, then the context.
func (a AppError) text() string {
	head := a.Message
	if a.Title != "" {
		if head != "" {
			head = a.Title + " — " + head
		} else {
			head = a.Title
		}
	}
	if a.Type != "" {
		if head != "" {
			head = a.Type + ": " + head
		} else {
			head = a.Type
		}
	}
	if head == "" {
		head = "(no message)"
	}
	parts := []string{clip(head, errorLogHeadMax)}
	if s := stackFrames(a.Stack); s != "" {
		parts = append(parts, clip(s, errorLogStackMax))
	}
	if a.Context != "" {
		parts = append(parts, "Context: "+clip(a.Context, errorLogContextMax))
	}
	return strings.Join(parts, "\n\n")
}

// stackFrames is a JS stack without its first line when that line is the
// error's own `Name: message`, which the row has already said, and without
// the frames of the prelude it starts with — the DDCoreError constructor,
// ddcore.throw — which are the same on every row and say nothing about the
// app's code.
func stackFrames(stack string) string {
	lines := strings.Split(strings.TrimRight(stack, "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "at ") {
			start = i
			break
		}
	}
	if start < 0 {
		return strings.Join(lines, "\n")
	}
	frames := lines[start:]
	for i, l := range frames {
		if !strings.Contains(l, "prelude.js:") {
			return strings.Join(frames[i:], "\n")
		}
	}
	return strings.Join(frames, "\n")
}

// clip cuts s to at most n bytes, on a rune boundary, and says so.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("… [%d bytes cut]", len(s)-cut)
}
