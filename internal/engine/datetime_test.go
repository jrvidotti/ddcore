package engine

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
)

// utcOpts is the coercion context a test uses when the timezone is not what it
// is asserting. `at(tz)` is for the tests that are.
var utcOpts = castOpts{loc: time.UTC, currencyPrec: 2, rounding: num.HalfAwayFromZero}

func at(t *testing.T, tz string) castOpts {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	o := utcOpts
	o.loc = loc
	return o
}

func TestCastValueDatetimeAceitaString(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	got, err := castValueWith(f, "2026-09-10 17:35:20", utcOpts)
	if err != nil {
		t.Fatalf("castValueWith() erro = %v", err)
	}
	if _, ok := got.(time.Time); !ok {
		t.Fatalf("castValueWith() = %#v, queria time.Time", got)
	}
}

// castAll grava o valor convertido de volta no documento: coagir de novo o
// mesmo campo não pode falhar (era o caso quando o app gravava duas vezes).
func TestCastValueDatetimeEhIdempotente(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	primeiro, err := castValueWith(f, "2026-09-10 17:35:20", utcOpts)
	if err != nil {
		t.Fatalf("castValueWith() erro = %v", err)
	}
	segundo, err := castValueWith(f, primeiro, utcOpts)
	if err != nil {
		t.Fatalf("segunda castValueWith() erro = %v", err)
	}
	if !segundo.(time.Time).Equal(primeiro.(time.Time)) {
		t.Fatalf("segunda castValueWith() = %v, queria %v", segundo, primeiro)
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

// A naked datetime string carries no offset, so something has to decide which
// clock wrote it. It used to be the process's — which meant `ddcore.utils.now()`,
// which returns the *site's* wall clock, was read back as if it had been the
// server's. Invisible while both are UTC, and an hours-long error the moment
// they are not.
func TestNakedDatetimeIsReadInTheSiteTimezone(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	for _, c := range []struct {
		tz   string
		want string
	}{
		{"UTC", "2026-01-15T10:00:00Z"},
		{"America/Sao_Paulo", "2026-01-15T13:00:00Z"},
		{"Asia/Tokyo", "2026-01-15T01:00:00Z"},
	} {
		got, err := castValueWith(f, "2026-01-15 10:00:00", at(t, c.tz))
		if err != nil {
			t.Fatalf("%s: %v", c.tz, err)
		}
		if s := got.(time.Time).UTC().Format(time.RFC3339); s != c.want {
			t.Errorf("%s: 10:00 local = %s, want %s", c.tz, s, c.want)
		}
	}
}

// An offset in the string is the string's own answer and must win over the
// site's, or a payload from another system would be silently relabelled.
func TestDatetimeWithAnOffsetKeepsIt(t *testing.T) {
	f := &meta.Field{Fieldname: "quando", Fieldtype: "Datetime", Label: "Quando"}
	got, err := castValueWith(f, "2026-01-15T10:00:00-05:00", at(t, "Asia/Tokyo"))
	if err != nil {
		t.Fatal(err)
	}
	if s := got.(time.Time).UTC().Format(time.RFC3339); s != "2026-01-15T15:00:00Z" {
		t.Fatalf("got %s, want 2026-01-15T15:00:00Z", s)
	}
}

// A Date is a civil date and never an instant: 2018-11-04 is the fourth of
// November everywhere, including on the day Brazil's local midnight did not
// exist because DST started at it.
func TestDateNeverSlidesByTimezone(t *testing.T) {
	f := &meta.Field{Fieldname: "emissao", Fieldtype: "Date", Label: "Emissão"}
	for _, tz := range []string{"UTC", "America/Sao_Paulo", "Asia/Tokyo", "Pacific/Kiritimati"} {
		for _, day := range []string{"2018-11-04", "2019-02-16", "2026-03-01", "1970-01-01"} {
			got, err := castValueWith(f, day, at(t, tz))
			if err != nil {
				t.Fatalf("%s %s: %v", tz, day, err)
			}
			if got != day {
				t.Errorf("%s: %s came back as %v", tz, day, got)
			}
		}
	}
}

// Time was the one fieldtype that validated nothing: any string went to
// Postgres, which answered with a driver error naming neither field nor value.
func TestTimeIsValidatedAndNormalised(t *testing.T) {
	f := &meta.Field{Fieldname: "hora", Fieldtype: "Time", Label: "Hora"}
	for in, want := range map[string]string{
		"09:30":        "09:30:00",
		"09:30:15":     "09:30:15",
		"  23:59:59  ": "23:59:59",
		"09:30:15.250": "09:30:15.25",
	} {
		got, err := castValueWith(f, in, utcOpts)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != want {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"25:00", "9h30", "abc", "2026-01-15"} {
		if _, err := castValueWith(f, bad, utcOpts); err == nil {
			t.Errorf("%q was accepted", bad)
		} else if got := cerr.From(err).Type; got != "ValidationError" {
			t.Errorf("%q gave %s, want ValidationError", bad, got)
		}
	}
}
