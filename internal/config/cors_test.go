package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestCORSOriginsFromFileAndEnvironment(t *testing.T) {
	clearMailEnv(t)
	f, _, err := Load(site(t, `{"cors":{"origins":["https://shop.example.com","https://*.partner.com"]}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{"https://shop.example.com", "https://*.partner.com"}; !reflect.DeepEqual(f.CORS.Origins, want) {
		t.Fatalf("origins = %v, want %v", f.CORS.Origins, want)
	}

	// the environment replaces the list; spaces and empty items are noise
	t.Setenv("DDCORE_CORS_ORIGINS", " https://a.com, http://localhost:5173 ,,")
	f, _, err = Load(site(t, `{"cors":{"origins":["https://shop.example.com"]}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{"https://a.com", "http://localhost:5173"}; !reflect.DeepEqual(f.CORS.Origins, want) {
		t.Fatalf("origins = %v, want %v", f.CORS.Origins, want)
	}

	t.Setenv("DDCORE_CORS_ORIGINS", "*")
	if f, _, err = Load(site(t, `{}`)); err != nil || !reflect.DeepEqual(f.CORS.Origins, []string{"*"}) {
		t.Fatalf("*: %v %v", f.CORS.Origins, err)
	}
}

func TestCORSOriginsRefuseWhatNoBrowserSends(t *testing.T) {
	clearMailEnv(t)
	for _, o := range []string{
		"shop.example.com",              // no scheme
		"https://shop.example.com/",     // a path: an Origin never has one
		"https://shop.example.com/pay",  // likewise
		"https://*",                     // a wildcard that is "*" with extra steps
		"https://a.*.com",               // wildcard only as the first label
		"https://shop.example.com:abc",  // port
		"ftp://shop.example.com",        // not a scheme a page is served over
		"https://user@shop.example.com", // credentials
	} {
		t.Run(o, func(t *testing.T) {
			t.Setenv("DDCORE_CORS_ORIGINS", o)
			_, _, err := Load(site(t, `{}`))
			if err == nil || !strings.Contains(err.Error(), "cors") {
				t.Fatalf("%q: expected a cors error, got %v", o, err)
			}
		})
	}
	for _, o := range []string{"*", "https://shop.example.com", "http://localhost:5173", "https://*.partner.com", "HTTPS://Shop.Example.com"} {
		t.Setenv("DDCORE_CORS_ORIGINS", o)
		if _, _, err := Load(site(t, `{}`)); err != nil {
			t.Fatalf("%q: %v", o, err)
		}
	}
}
