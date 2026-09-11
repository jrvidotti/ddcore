package engine

import (
	"strings"
	"unicode/utf8"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// Password policy has exactly one chokepoint, and it is the hash rather than
// any of the call sites.
//
// There are five ways a password gets set — the User form, `ddcore user add`,
// `ddcore user passwd`, a recovery token, and a person changing their own —
// and validating at each of them is five places to forget. All five end up
// producing a hash, so validating there catches every one, including app code
// nobody has written yet. The only way past it is writing password_hash
// directly, which is already a deliberate act.

// maxPasswordLength caps what reaches Argon2. Past this a "password" is a
// document, and hashing it is work an unauthenticated caller chose for us.
const maxPasswordLength = 128

// ValidatePassword applies the site's policy. identity is the user the
// password is for, when known: it is only used to refuse a password that is
// the account name, which is the one guess every attacker makes first.
func (e *Engine) ValidatePassword(identity, pw string) error {
	min := e.Cfg.Auth.MinPasswordLength
	if min <= 0 {
		min = 8
	}
	// Count runes, not bytes: "senhaç" is six characters to the person who
	// typed it, and byte-counting would quietly reward accented alphabets.
	if utf8.RuneCountInString(pw) < min {
		return cerr.Validation("The password must have at least {0} characters", min)
	}
	if utf8.RuneCountInString(pw) > maxPasswordLength {
		return cerr.Validation("The password must have at most {0} characters", maxPasswordLength)
	}
	if strings.TrimSpace(pw) == "" {
		return cerr.Validation("The password cannot be blank")
	}
	if identity != "" && strings.EqualFold(strings.TrimSpace(pw), strings.TrimSpace(identity)) {
		return cerr.Validation("The password cannot be the same as the username")
	}
	return nil
}

// HashNewPassword validates and then hashes. Every path that sets a password
// goes through here; CheckPassword's counterpart HashPassword stays unchecked
// because it is also how the decoy and the fixtures are built.
func (e *Engine) HashNewPassword(identity, pw string) (string, error) {
	if err := e.ValidatePassword(identity, pw); err != nil {
		return "", err
	}
	return HashPassword(pw), nil
}
