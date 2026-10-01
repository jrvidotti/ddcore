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
		{
			name: "a library the site's app requires does not name the site",
			st: func() *State {
				// dependency order puts the library first, whatever ddcore.json says
				st := state([2]string{"core", "DDCore"}, [2]string{"lib", "Lib"}, [2]string{"site", "My Site"})
				st.Snap.Apps["site"].Requires = []string{"lib"}
				return st
			}(),
			want:  "My Site",
			about: "a required app is a library; the app nobody requires is the site's own",
		},
		{
			name: "a title-less site app leaves the name to its library",
			st: func() *State {
				st := state([2]string{"core", "DDCore"}, [2]string{"lib", "Lib"}, [2]string{"site", ""})
				st.Snap.Apps["site"].Requires = []string{"lib"}
				return st
			}(),
			want:  "Lib",
			about: "a library's title still beats the core's",
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

// TestSiteDesk: desk.home and desk.logo are the site's in the same order as its
// name, each key on its own — the first app in that order that declares it.
func TestSiteDesk(t *testing.T) {
	st := state([2]string{"core", "DDCore"}, [2]string{"lib", "Lib"}, [2]string{"site", "My Site"}, [2]string{"other", "Other"})
	st.Snap.Apps["site"].Requires = []string{"lib"}
	st.Snap.Apps["lib"].Desk = map[string]any{"home": "Lib Home", "logo": "L"}
	st.Snap.Apps["site"].Desk = map[string]any{"home": "Site Home"}
	st.Snap.Apps["other"].Desk = map[string]any{"home": "Other Home"}

	if got := st.SiteDesk("home"); got != "Site Home" {
		t.Errorf("SiteDesk(home) = %q, expected the site app's, ahead of its library and of a later app", got)
	}
	if got := st.SiteDesk("logo"); got != "L" {
		t.Errorf("SiteDesk(logo) = %q, expected the library's, the only one declared", got)
	}
	if got := st.SiteDesk("include"); got != "" {
		t.Errorf("SiteDesk(include) = %q, expected nothing for a key no app declares as a string", got)
	}
}
