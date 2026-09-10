package engine

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestCastValueDatetimeAceitaString(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	got, err := castValue(f, "2026-09-10 17:35:20")
	if err != nil {
		t.Fatalf("castValue() erro = %v", err)
	}
	if _, ok := got.(time.Time); !ok {
		t.Fatalf("castValue() = %#v, queria time.Time", got)
	}
}

// castAll grava o valor convertido de volta no documento: coagir de novo o
// mesmo campo não pode falhar (era o caso quando o app gravava duas vezes).
func TestCastValueDatetimeEhIdempotente(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	primeiro, err := castValue(f, "2026-09-10 17:35:20")
	if err != nil {
		t.Fatalf("castValue() erro = %v", err)
	}
	segundo, err := castValue(f, primeiro)
	if err != nil {
		t.Fatalf("segunda castValue() erro = %v", err)
	}
	if !segundo.(time.Time).Equal(primeiro.(time.Time)) {
		t.Fatalf("segunda castValue() = %v, queria %v", segundo, primeiro)
	}
}

// TestTodayFollowsSiteTimezone: `today()` is the site's day on both sides of
// the wire — that is what makes `due_date < today()` give one answer.
func TestTodayFollowsSiteTimezone(t *testing.T) {
	for _, tz := range []string{"UTC", "America/Sao_Paulo", "Asia/Tokyo"} {
		e := &Engine{Cfg: Config{Timezone: tz}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.Location().String(); got != loc.String() {
			t.Fatalf("Location() = %q, want %q", got, tz)
		}
		c := &Ctx{E: e}
		if got, want := c.Today(), time.Now().In(loc).Format("2006-01-02"); got != want {
			t.Fatalf("%s: Today() = %q, want %q", tz, got, want)
		}
	}
}

// An unknown zone name falls back to UTC, never to the machine's local time.
func TestUnknownTimezoneFallsBackToUTC(t *testing.T) {
	e := &Engine{Cfg: Config{Timezone: "Mars/Olympus"}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if got := e.Location(); got != time.UTC {
		t.Fatalf("Location() = %v, want UTC", got)
	}
}
