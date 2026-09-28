package config

import (
	"strings"
	"testing"
)

// Without a setting the map draws OpenStreetMap's tiles with their credit; a
// provider of one's own comes with its own credit or none — the OSM one is
// never put on someone else's tiles.
func TestMapTilesFromEnv(t *testing.T) {
	clearMailEnv(t)
	f, _, err := Load(site(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Map.TileURL != DefaultMapTileURL || f.Map.Attribution != DefaultMapAttribution {
		t.Fatalf("defaults: %+v", f.Map)
	}

	t.Setenv("DDCORE_MAP_ATTRIBUTION", "Map data © OSM")
	if f, _, err = Load(site(t, `{}`)); err != nil || f.Map.Attribution != "Map data © OSM" {
		t.Fatalf("the default tiles' credit can be reworded: %+v %v", f.Map, err)
	}

	clearMailEnv(t)
	t.Setenv("DDCORE_MAP_TILE_URL", "https://tiles.example.com/{z}/{x}/{y}.png?key=k")
	if f, _, err = Load(site(t, `{}`)); err != nil {
		t.Fatal(err)
	}
	if f.Map.TileURL != "https://tiles.example.com/{z}/{x}/{y}.png?key=k" || f.Map.Attribution != "" {
		t.Fatalf("another provider: %+v", f.Map)
	}
	t.Setenv("DDCORE_MAP_ATTRIBUTION", `<a href="https://example.com">Example Maps</a>`)
	if f, _, err = Load(site(t, `{}`)); err != nil || !strings.Contains(f.Map.Attribution, "Example Maps") {
		t.Fatalf("another provider's credit: %+v %v", f.Map, err)
	}

	for _, bad := range []string{"tiles.example.com/{z}/{x}/{y}.png", "javascript:alert(1)//{z}{x}{y}", "https://tiles.example.com/{z}/{x}.png"} {
		t.Setenv("DDCORE_MAP_TILE_URL", bad)
		if _, _, err := Load(site(t, `{}`)); err == nil || !strings.Contains(err.Error(), "DDCORE_MAP_TILE_URL") {
			t.Errorf("%q should be refused: %v", bad, err)
		}
	}

	// an engine built by hand, with no Load, still gets a map
	if m := (MapTiles{}).WithDefaults(); m.TileURL != DefaultMapTileURL || m.Attribution != DefaultMapAttribution {
		t.Fatalf("WithDefaults: %+v", m)
	}
}
