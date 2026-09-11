// Package num holds the framework's numeric contract: how many decimal places
// a money value has, and how a value is rounded to them.
//
// Two decisions shape everything here.
//
// First, rounding is defined over the value's *shortest round-trip decimal
// representation*, not over its binary double. A user who types 1.005 stores
// the double 1.00499999999999989; rounding the binary value gives 1.00, which
// is not what anyone looking at the form will accept. Formatting the double
// back to its shortest decimal recovers "1.005" — the number the person meant
// — and rounding that string gives 1.01.
//
// Second, the algorithm has to be reproducible outside Go. The same rule runs
// in the app runtime (internal/js/prelude.js) and in the desk
// (desk/src/lib/round.ts), because a total computed in a form script and the
// total the server stores have to agree. JavaScript's String(v) produces
// exactly the digits strconv.FormatFloat(v, 'f', -1, 64) does, so a
// string-based algorithm is identical in all three by construction, where
// anything built on powers of ten is not.
package num

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/text/currency"
)

// Rounding is what to do with a value exactly halfway between two results.
type Rounding int

const (
	// HalfAwayFromZero is the commercial rule and the default: 2.345 → 2.35,
	// -2.345 → -2.35. It is symmetric, which is what makes a credit and a
	// debit of the same magnitude cancel.
	HalfAwayFromZero Rounding = iota
	// HalfToEven is banker's rounding: 2.345 → 2.34, 2.355 → 2.36. It removes
	// the upward bias of summing many rounded values, and it is what a Frappe
	// site configured that way reconciles against.
	HalfToEven
)

func (r Rounding) String() string {
	if r == HalfToEven {
		return "bankers"
	}
	return "commercial"
}

// ParseRounding reads the `rounding` key of ddcore.json.
func ParseRounding(s string) (Rounding, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "commercial":
		return HalfAwayFromZero, nil
	case "bankers", "banker's":
		return HalfToEven, nil
	}
	return HalfAwayFromZero, fmt.Errorf("unknown rounding %q: use \"commercial\" or \"bankers\"", s)
}

// Round rounds v to p decimal places under rule r.
//
// A negative p, or a value that is not finite, is returned untouched: the
// caller decides whether that is an error, and rounding is not the place to
// invent a number.
func Round(v float64, p int, r Rounding) float64 {
	if p < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac, _ := strings.Cut(s, ".")
	if len(frac) <= p {
		return v // already exact at p places
	}
	digits := intPart + frac[:p]
	rest := frac[p:]

	if roundsUp(digits, rest, r) {
		digits = increment(digits)
	}
	out := insertPoint(digits, p)
	if neg {
		out = "-" + out
	}
	f, err := strconv.ParseFloat(out, 64)
	if err != nil {
		return v
	}
	if f == 0 {
		return 0 // never hand back a negative zero
	}
	return f
}

// roundsUp decides on the discarded digits alone. Everything before the
// half-way case is exact: a leading digit above or below 5 settles it, and a
// leading 5 followed by anything non-zero is above half, not at it.
func roundsUp(digits, rest string, r Rounding) bool {
	if rest == "" || rest[0] < '5' {
		return false
	}
	if rest[0] > '5' || strings.Trim(rest[1:], "0") != "" {
		return true
	}
	if r == HalfToEven {
		last := byte('0')
		if len(digits) > 0 {
			last = digits[len(digits)-1]
		}
		return (last-'0')%2 == 1
	}
	return true
}

// increment adds one to a decimal digit string, carrying into a new leading
// digit when it has to ("999" → "1000").
func increment(d string) string {
	b := []byte(d)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}

// insertPoint puts the decimal point p digits from the right, padding with
// leading zeros when the carry did not reach that far (".05" at p=2 is "005").
func insertPoint(d string, p int) string {
	if p == 0 {
		return d
	}
	for len(d) <= p {
		d = "0" + d
	}
	return d[:len(d)-p] + "." + d[len(d)-p:]
}

// MinorUnits is how many decimal places a currency has: 2 for USD and BRL, 0
// for JPY, 3 for BHD. An unrecognised code falls back to 2 rather than to 0,
// because losing the cents of an unknown currency is the worse failure.
func MinorUnits(code string) int {
	u, err := currency.ParseISO(strings.TrimSpace(code))
	if err != nil {
		return 2
	}
	scale, _ := currency.Standard.Rounding(u)
	return scale
}
