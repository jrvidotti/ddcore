package engine

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The core/app compatibility contract (PRD-07). An app declares the range of
// ddcore releases it was written against — `ddcore: ">=0.14.0 <1.0.0"` in
// defineApp — and a binary outside that range refuses to load it, instead of
// failing later on a hook that changed shape or a method that no longer exists.
//
// The range grammar is deliberately small: whitespace-separated constraints
// that must all hold, each one `>=`, `>`, `<=`, `<`, `=` or a bare version, or
// `^X.Y.Z` / `~X.Y.Z` with npm's meaning. Anything else is a load error: a range
// that silently matched everything would be worse than no range at all.

// semver is a release number; pre-release and build metadata are not part of
// the contract, so they are not represented.
type semver [3]int

func (v semver) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

func (v semver) cmp(o semver) int {
	for i := range v {
		if v[i] != o[i] {
			if v[i] < o[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

var versionRe = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)

// parseVersion reads `1`, `1.2`, `1.2.3`, with or without a leading `v`.
func parseVersion(s string) (semver, bool) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return semver{}, false
	}
	var v semver
	for i := 0; i < 3; i++ {
		if m[i+1] != "" {
			n, err := strconv.Atoi(m[i+1])
			if err != nil {
				return semver{}, false
			}
			v[i] = n
		}
	}
	return v, true
}

var coreVersionRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(?:-.*)?$`)

// parseCoreVersion reads the version the binary was built with. A release tag
// (`v0.14.0`) and a `git describe` build on top of one (`v0.14.0-3-gabc-dirty`)
// both count as that release; `dev`, `latest` or a bare hash are not releases,
// and ok is false.
func parseCoreVersion(s string) (semver, bool) {
	m := coreVersionRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return semver{}, false
	}
	return parseVersion(m[1])
}

type constraint struct {
	op string // >=, >, <=, <, =
	v  semver
}

// versionRange is a conjunction of constraints.
type versionRange []constraint

func (r versionRange) allows(v semver) bool {
	for _, c := range r {
		d := v.cmp(c.v)
		ok := false
		switch c.op {
		case ">=":
			ok = d >= 0
		case ">":
			ok = d > 0
		case "<=":
			ok = d <= 0
		case "<":
			ok = d < 0
		case "=":
			ok = d == 0
		}
		if !ok {
			return false
		}
	}
	return true
}

func parseRange(s string) (versionRange, error) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty range")
	}
	var r versionRange
	for _, p := range parts {
		op := ""
		for _, o := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
			if strings.HasPrefix(p, o) {
				op, p = o, strings.TrimPrefix(p, o)
				break
			}
		}
		v, ok := parseVersion(p)
		if !ok {
			return nil, fmt.Errorf("%q is not a version", p)
		}
		switch op {
		case "":
			r = append(r, constraint{"=", v})
		case "^":
			// npm: the leftmost non-zero part is fixed
			upper := semver{v[0] + 1, 0, 0}
			if v[0] == 0 {
				upper = semver{0, v[1] + 1, 0}
			}
			r = append(r, constraint{">=", v}, constraint{"<", upper})
		case "~":
			r = append(r, constraint{">=", v}, constraint{"<", semver{v[0], v[1] + 1, 0}})
		default:
			r = append(r, constraint{op, v})
		}
	}
	return r, nil
}

// checkCoreCompat refuses a load where an app declares a ddcore range this
// binary is outside of, or a range or version that does not parse. Every
// offending app is named at once, so an upgrade shows the whole list.
//
// A binary that is not a release (`dev`, `latest`) cannot be compared: ranges
// are still parsed, so a typo fails on a dev checkout too, but not enforced,
// and skipped reports that.
func checkCoreCompat(snap *Snapshot, core string) (skipped bool, err error) {
	cv, release := parseCoreVersion(core)
	names := make([]string, 0, len(snap.Apps))
	for n := range snap.Apps {
		names = append(names, n)
	}
	sort.Strings(names)
	var problems []string
	for _, n := range names {
		am := snap.Apps[n]
		if am == nil {
			continue
		}
		if am.Version != "" {
			if _, ok := parseVersion(am.Version); !ok {
				problems = append(problems, fmt.Sprintf("app %s declares version %q, which is not MAJOR.MINOR.PATCH", n, am.Version))
			}
		}
		if am.Ddcore == "" {
			continue
		}
		r, perr := parseRange(am.Ddcore)
		if perr != nil {
			problems = append(problems, fmt.Sprintf("app %s declares an invalid ddcore range %q: %v", n, am.Ddcore, perr))
			continue
		}
		if !release {
			skipped = true
			continue
		}
		if !r.allows(cv) {
			problems = append(problems, fmt.Sprintf("app %s requires ddcore %s, but this binary is %s", n, am.Ddcore, cv))
		}
	}
	if len(problems) > 0 {
		return skipped, fmt.Errorf("incompatible apps: %s", strings.Join(problems, "; "))
	}
	return skipped, nil
}
