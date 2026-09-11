package engine

import (
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// state builds the two fields SiteTitle reads: the apps in load order, and the
// metadata each one produced. Core is prepended the way Engine.Load does it.
func state(titles ...[2]string) *State {
	st := &State{Snap: &Snapshot{Apps: map[string]*AppMeta{}}}
	for _, t := range titles {
		name, title := t[0], t[1]
		st.Apps = append(st.Apps, js.App{Name: name})
		st.Snap.Apps[name] = &AppMeta{Name: name, Title: title}
	}
	return st
}

func TestSiteTitle(t *testing.T) {
	cases := []struct {
		name  string
		st    *State
		want  string
		about string
	}{
		{
			name:  "the app names the site, not the core",
			st:    state([2]string{"core", "DDCore"}, [2]string{"demo", "Projects"}),
			want:  "Projects",
			about: "the desk is showing the app's screens, so it carries the app's name",
		},
		{
			name:  "the first app wins, as desk.home does",
			st:    state([2]string{"core", "DDCore"}, [2]string{"billing", "Billing"}, [2]string{"crm", "CRM"}),
			want:  "Billing",
			about: "load order is the tie-break everywhere else in the desk block",
		},
		{
			name:  "a site with no app of its own falls back to the core",
			st:    state([2]string{"core", "DDCore"}),
			want:  "DDCore",
			about: "a bare ddcore still has to call itself something",
		},
		{
			name:  "an app that declared no title is skipped, not shown blank",
			st:    state([2]string{"core", "DDCore"}, [2]string{"quiet", ""}),
			want:  "DDCore",
			about: "title is required by the SDK type but not by the Go side",
		},
		{
			name: "an app with no metadata at all is skipped",
			st: func() *State {
				st := state([2]string{"core", "DDCore"})
				st.Apps = append(st.Apps, js.App{Name: "broken"})
				return st
			}(),
			want:  "DDCore",
			about: "an app that failed to produce meta must not blank the heading",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.st.SiteTitle(); got != c.want {
				t.Errorf("SiteTitle() = %q, expected %q — %s", got, c.want, c.about)
			}
		})
	}
}
