package api

import (
	"io"
	"net/http"
	"sync"
	"testing"
)

// Boot and getMeta read the state their request captured, never one a reload
// is replacing: `make test-race` runs this under the race detector.
func TestBootAndMetaDuringReload(t *testing.T) {
	x := setup(t)
	sid := x.sid("Admin")
	get := func(path string) (int, string, error) {
		req, _ := http.NewRequest("GET", x.ts.URL+path, nil)
		req.Header.Set("X-Requested-With", "test")
		req.AddCookie(&http.Cookie{Name: "sid", Value: sid})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(raw), nil
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, path := range []string{"/api/boot", "/api/meta/Pessoa", "/api/boot", "/api/meta/Pessoa"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				status, raw, err := get(path)
				if err != nil || status != http.StatusOK {
					t.Errorf("%s during a reload: %d %v %s", path, status, err, raw)
					return
				}
			}
		}()
	}
	for range 5 {
		if err := x.e.Load(); err != nil {
			t.Error(err)
			break
		}
	}
	close(done)
	wg.Wait()
}
