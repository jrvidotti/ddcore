package config

import (
	"fmt"
	"net/mail"
	"os"
	"strings"
)

// Feedback is what the desk's Feedback dialog does: whether users see it, and
// who is mailed when one arrives. It is the site's decision, so it lives in
// ddcore.json; DDCORE_FEEDBACK and DDCORE_FEEDBACK_TO override it where a
// deployment differs.
type Feedback struct {
	// Enabled is nil when ddcore.json says nothing, which means on.
	Enabled *bool `json:"enabled,omitempty"`
	// To are the addresses mailed a copy of each feedback, attachments
	// included. Empty, nobody is mailed; the platform's System Managers still
	// get it in their desk inbox.
	To []string `json:"to,omitempty"`
}

// On is whether the feature is offered: the dialog, its endpoints, the boot flag.
func (f Feedback) On() bool { return f.Enabled == nil || *f.Enabled }

// fromEnv applies DDCORE_FEEDBACK (on/off) and DDCORE_FEEDBACK_TO
// (comma-separated, replacing the file's list) over the file.
func (f *Feedback) fromEnv() {
	if strings.TrimSpace(os.Getenv("DDCORE_FEEDBACK")) != "" {
		on := envBool("DDCORE_FEEDBACK", f.On())
		f.Enabled = &on
	}
	v := strings.TrimSpace(os.Getenv("DDCORE_FEEDBACK_TO"))
	if v == "" {
		return
	}
	f.To = nil
	for _, a := range strings.Split(v, ",") {
		if a = strings.TrimSpace(a); a != "" {
			f.To = append(f.To, a)
		}
	}
}

// validate refuses an address no mail server would take: a typo would
// otherwise only show as a Failed delivery nobody looks at.
func (f Feedback) validate() error {
	for _, a := range f.To {
		if addr, err := mail.ParseAddress(a); err != nil || addr.Name != "" {
			return fmt.Errorf("feedback.to: %q is not an email address", a)
		}
	}
	return nil
}
