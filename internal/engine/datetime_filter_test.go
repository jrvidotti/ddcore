package engine

import (
	"context"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/js"
)

var datetimeFilterFiles = map[string]string{
	"doctypes/evento_hora/evento_hora.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Evento Hora", isChild: true, fields: [
  { fieldname: "quando", fieldtype: "Datetime", label: "When" } ] });`,
	"doctypes/evento/evento.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Evento", fields: [
  { fieldname: "titulo", fieldtype: "Data", label: "Title" },
  { fieldname: "at", fieldtype: "Datetime", label: "At" },
  { fieldname: "horas", fieldtype: "Table", label: "Hours", options: "Evento Hora" } ] });`,
}

// A Datetime filter without an offset is read on the site's clock, as a write
// is, not on the connection's UTC (#125). The site is three hours behind UTC,
// so every bound below would miss its row if it were read as UTC.
func TestDatetimeFilterReadsSiteClock(t *testing.T) {
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, datetimeFilterFiles)}}, Test: true,
		Timezone: "America/Sao_Paulo"})
	loc := e.Location()
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, at := range []string{"2026-01-15 10:00:00", "2026-01-15 22:00:00"} {
			n, _ := c.NewDoc("Evento", Doc{"titulo": at, "at": at, "horas": []any{map[string]any{"quando": at}}})
			if _, err := c.Insert(n, SaveOpts{}); err != nil {
				return err
			}
		}
		// after the inserts, rounded up past their sub-second creation
		now := time.Now().In(loc).Add(time.Second).Format("2006-01-02 15:04:05")
		hourAgo := time.Now().In(loc).Add(-time.Hour).Format("2006-01-02 15:04:05")
		cases := []struct {
			filters any
			want    int64
		}{
			{map[string]any{"at": "2026-01-15 10:00:00"}, 1},
			{map[string]any{"at": "2026-01-15T13:00:00Z"}, 1}, // an offset keeps its instant
			{[]any{[]any{"at", "!=", "2026-01-15 10:00:00"}}, 1},
			{[]any{[]any{"at", "<", "2026-01-15 10:30"}}, 1},
			{[]any{[]any{"at", ">", "2026-01-15 21:30:00"}}, 1},
			{[]any{[]any{"at", "between", []any{"2026-01-15 09:00:00", "2026-01-15 11:00:00"}}}, 1},
			{[]any{[]any{"at", "in", []any{"2026-01-15 10:00:00", "2026-01-15 22:00:00"}}}, 2},
			{[]any{[]any{"at", "not in", "2026-01-15 10:00:00"}}, 1},
			// a date alone is midnight on the site's clock: 22:00 local is the
			// next day in UTC, still before the 16th here
			{[]any{[]any{"at", "<", "2026-01-16"}}, 2},
			{[]any{[]any{"at", ">=", "2026-01-15"}}, 2},
			{[]any{[]any{"Evento Hora.quando", "<=", "2026-01-15 10:00:00"}}, 1},
			{[]any{[]any{"creation", "<=", now}}, 2},
			{[]any{[]any{"creation", "<", hourAgo}}, 0},
			{[]any{[]any{"modified", ">=", hourAgo}}, 2},
			{[]any{[]any{"Evento Hora.creation", "<=", now}}, 2},
		}
		for _, tc := range cases {
			got, err := c.Count("Evento", tc.filters)
			if err != nil {
				t.Fatalf("Count(%v): %v", tc.filters, err)
			}
			if got != tc.want {
				t.Errorf("Count(%v) = %d, want %d", tc.filters, got, tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
