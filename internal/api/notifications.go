package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

func notificationQuery(r *http.Request, allowed ...string) error {
	for key, values := range r.URL.Query() {
		ok := false
		for _, candidate := range allowed {
			ok = ok || key == candidate
		}
		if !ok || len(values) != 1 {
			return cerr.Validation("Invalid {0}: {1}", key, "unsupported or repeated parameter")
		}
	}
	return nil
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := notificationQuery(r, "limit", "offset", "read"); err != nil {
			return nil, err
		}
		q := r.URL.Query()
		limit, start := 20, 0
		for key, target := range map[string]*int{"limit": &limit, "offset": &start} {
			if q.Has(key) {
				n, err := strconv.Atoi(q.Get(key))
				if err != nil || n < 0 || (key == "limit" && (n == 0 || n > 100)) {
					return nil, cerr.Validation("Invalid {0}: {1}", key, "out of range")
				}
				*target = n
			}
		}
		var read *bool
		if q.Has("read") {
			value := q.Get("read")
			if value != "true" && value != "false" {
				return nil, cerr.Validation("Invalid {0}: {1}", "read", "expected true or false")
			}
			v := value == "true"
			read = &v
		}
		return c.ListNotifications(limit, start, read)
	})
}

func (s *Server) notificationCount(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := notificationQuery(r); err != nil {
			return nil, err
		}
		return c.NotificationCount()
	})
}

func (s *Server) setNotificationRead(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := notificationQuery(r); err != nil {
			return nil, err
		}
		var body struct {
			Read *bool `json:"read"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			return nil, cerr.Validation("Invalid JSON: {0}", err)
		}
		if body.Read == nil {
			return nil, cerr.Validation("Invalid {0}: {1}", "read", "required boolean")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, cerr.Validation("Invalid JSON: {0}", "expected one object")
		}
		return c.SetNotificationRead(urlParam(r, "id"), *body.Read)
	})
}
