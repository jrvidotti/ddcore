package config

import (
	"fmt"
	"strings"
)

// DefaultMapTileURL is OpenStreetMap's own tile server. It is fine for
// development and a small internal site; its usage policy
// (https://operations.osmfoundation.org/policies/tiles/) rules out heavy use,
// so a production deployment points DDCORE_MAP_TILE_URL at a provider of its
// own.
const DefaultMapTileURL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"

// DefaultMapAttribution is what the default tiles require to be shown.
const DefaultMapAttribution = "© OpenStreetMap contributors"

// MapTiles is where a Geolocation field's map draws its tiles from, handed to
// the desk in the boot payload. The browser fetches the tiles; the server
// never does.
//
// Environment and not ddcore.json: the tile provider, and the key a paid one
// puts in the URL, differ by deployment — development on the public OSM
// servers, production on a provider under contract.
type MapTiles struct {
	// TileURL is a Leaflet URL template with {z}, {x} and {y}, and optionally
	// {s} and {r}.
	TileURL string
	// Attribution is the credit the provider requires, shown in the map's
	// corner. It may hold a link; the desk sanitizes it.
	Attribution string
}

// WithDefaults fills an empty TileURL with OpenStreetMap's, and its
// attribution — an engine built by hand gets a working map.
func (m MapTiles) WithDefaults() MapTiles {
	if m.TileURL == "" {
		return MapTiles{TileURL: DefaultMapTileURL, Attribution: DefaultMapAttribution}
	}
	return m
}

func mapTilesFromEnv() (MapTiles, error) {
	url := strings.TrimSpace(env("DDCORE_MAP_TILE_URL", ""))
	if url == "" {
		// the default tiles come with their credit, which can still be
		// reworded; another provider's credit is not ours to guess
		return MapTiles{TileURL: DefaultMapTileURL, Attribution: env("DDCORE_MAP_ATTRIBUTION", DefaultMapAttribution)}, nil
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return MapTiles{}, fmt.Errorf("DDCORE_MAP_TILE_URL: %q is not an http(s) URL", url)
	}
	for _, p := range []string{"{z}", "{x}", "{y}"} {
		if !strings.Contains(url, p) {
			return MapTiles{}, fmt.Errorf("DDCORE_MAP_TILE_URL: %q has no %s", url, p)
		}
	}
	return MapTiles{TileURL: url, Attribution: strings.TrimSpace(env("DDCORE_MAP_ATTRIBUTION", ""))}, nil
}
