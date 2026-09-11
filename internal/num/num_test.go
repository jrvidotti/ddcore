package num

import (
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// Case is one row of testdata/rounding.json. The same file is read by the app
// runtime's tests (internal/js) and by the desk's (desk/src/lib/round.test.ts):
// the three implementations are only a contract if they are asserted against
// the same numbers.
type Case struct {
	Why        string  `json:"why"`
	V          float64 `json:"v"`
	P          int     `json:"p"`
	Commercial float64 `json:"commercial"`
	Bankers    float64 `json:"bankers"`
}

func vectors(t *testing.T) []Case {
	t.Helper()
	b, err := os.ReadFile("testdata/rounding.json")
	if err != nil {
		t.Fatalf("reading the vectors: %v", err)
	}
	var f struct {
		Cases []Case `json:"cases"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("parsing the vectors: %v", err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("no vectors")
	}
	return f.Cases
}

func TestRoundVectors(t *testing.T) {
	for _, c := range vectors(t) {
		for _, m := range []struct {
			name string
			r    Rounding
			want float64
		}{
			{"commercial", HalfAwayFromZero, c.Commercial},
			{"bankers", HalfToEven, c.Bankers},
		} {
			got := Round(c.V, c.P, m.r)
			if got != m.want {
				t.Errorf("Round(%v, %d, %s) = %v, want %v — %s", c.V, c.P, m.name, got, m.want, c.Why)
			}
			if math.Signbit(got) && got == 0 {
				t.Errorf("Round(%v, %d, %s) returned negative zero", c.V, c.P, m.name)
			}
		}
	}
}

// Rounding an already-rounded value has to be a no-op. Without it, castAll —
// which runs twice per save — could move a value on the second pass, and a
// submitted document would refuse its own unchanged field.
func TestRoundIsIdempotent(t *testing.T) {
	for _, c := range vectors(t) {
		for _, r := range []Rounding{HalfAwayFromZero, HalfToEven} {
			once := Round(c.V, c.P, r)
			if twice := Round(once, c.P, r); twice != once {
				t.Errorf("Round(Round(%v, %d)) = %v, want %v", c.V, c.P, twice, once)
			}
		}
	}
}

func TestRoundLeavesWhatItCannotRepresent(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		got := Round(v, 2, HalfAwayFromZero)
		if !math.IsNaN(v) && got != v {
			t.Errorf("Round(%v) = %v, want it untouched", v, got)
		}
		if math.IsNaN(v) && !math.IsNaN(got) {
			t.Errorf("Round(NaN) = %v, want NaN", got)
		}
	}
	if got := Round(1.005, -1, HalfAwayFromZero); got != 1.005 {
		t.Errorf("a negative precision must not round: got %v", got)
	}
}

func TestMinorUnits(t *testing.T) {
	for _, c := range []struct {
		code string
		want int
	}{
		{"USD", 2}, {"BRL", 2}, {"EUR", 2},
		{"JPY", 0}, {"KWD", 3}, {"BHD", 3},
		{"", 2}, {"XYZ", 2}, // an unknown code keeps its cents rather than losing them
	} {
		if got := MinorUnits(c.code); got != c.want {
			t.Errorf("MinorUnits(%q) = %d, want %d", c.code, got, c.want)
		}
	}
}

func TestParseRounding(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Rounding
		ok   bool
	}{
		{"", HalfAwayFromZero, true},
		{"commercial", HalfAwayFromZero, true},
		{"bankers", HalfToEven, true},
		{"  Bankers ", HalfToEven, true},
		{"half-up", HalfAwayFromZero, false},
		{"nearest", HalfAwayFromZero, false},
	} {
		got, err := ParseRounding(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseRounding(%q) error = %v, want ok=%v", c.in, err, c.ok)
		}
		if c.ok && got != c.want {
			t.Errorf("ParseRounding(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// roundExact is an independent reference: it does the same job with exact
// rational arithmetic instead of digit strings. The point of keeping two
// implementations is that the fast one is checked against the obvious one over
// values no hand-written table would think of.
func roundExact(v float64, p int, r Rounding) float64 {
	rat, ok := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	if !ok {
		return v
	}
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(p)), nil))
	rat.Mul(rat, scale)

	neg := rat.Sign() < 0
	rat.Abs(rat)
	q, rem := new(big.Int).QuoRem(rat.Num(), rat.Denom(), new(big.Int))

	twice := new(big.Int).Lsh(rem, 1) // 2*rem vs denom decides the half
	switch twice.Cmp(rat.Denom()) {
	case 1:
		q.Add(q, big.NewInt(1))
	case 0:
		if r == HalfAwayFromZero || q.Bit(0) == 1 {
			q.Add(q, big.NewInt(1))
		}
	}
	out, _ := new(big.Rat).SetFrac(q, scale.Num()).Float64()
	if neg {
		out = -out
	}
	if out == 0 {
		return 0
	}
	return out
}

func TestRoundMatchesExactArithmetic(t *testing.T) {
	rnd := rand.New(rand.NewSource(20260910))
	for i := 0; i < 20000; i++ {
		// magnitudes a ledger actually holds, down to sub-cent noise
		v := (rnd.Float64() - 0.5) * math.Pow(10, float64(rnd.Intn(9)-2))
		p := rnd.Intn(5)
		for _, r := range []Rounding{HalfAwayFromZero, HalfToEven} {
			got, want := Round(v, p, r), roundExact(v, p, r)
			if got != want {
				t.Fatalf("Round(%v, %d, %v) = %v, exact arithmetic says %v", v, p, r, got, want)
			}
		}
	}
}
