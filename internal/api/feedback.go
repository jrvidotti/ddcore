package api

import (
	"encoding/json"
	"mime/multipart"
	"net/http"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// feedbackAuthor reads who is sending, in the space the request works in.
func (s *Server) feedbackAuthor(r *http.Request) (engine.FeedbackAuthor, error) {
	var author engine.FeedbackAuthor
	_, _, err := s.execute(r, nil, func(c *engine.Ctx) (any, error) {
		a, err := c.FeedbackAuthorOf()
		author = a
		return nil, err
	})
	return author, err
}

// submitFeedback takes the desk's Feedback dialog: a `data` field holding the
// fields as JSON, and up to engine.FeedbackMaxFiles `files`, which together
// stay under the upload limit. The write itself is the engine's, in the
// platform space, so it does not run on this request's transaction.
func (s *Server) submitFeedback(w http.ResponseWriter, r *http.Request) {
	if !s.E.Cfg.Feedback.On() {
		http.NotFound(w, r)
		return
	}
	max := s.MaxUpload
	if max <= 0 {
		max = engine.DefaultMaxUpload
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.writeErr(w, r, cerr.Validation("Invalid upload: {0}", err))
		return
	}
	defer r.MultipartForm.RemoveAll()
	var data map[string]any
	if err := json.Unmarshal([]byte(r.FormValue("data")), &data); err != nil || data == nil {
		s.writeErr(w, r, cerr.Validation("Invalid {0}: {1}", "data", "expected a JSON object"))
		return
	}
	author, err := s.feedbackAuthor(r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) > engine.FeedbackMaxFiles {
		s.writeErr(w, r, cerr.Validation("At most {0} files can be attached", engine.FeedbackMaxFiles).WithTitleKey("Too many files"))
		return
	}
	files := make([]engine.FeedbackFile, 0, len(headers))
	for _, h := range headers {
		f, err := h.Open()
		if err != nil {
			s.writeErr(w, r, cerr.Validation("Invalid upload: {0}", err))
			return
		}
		defer func(f multipart.File) { f.Close() }(f)
		files = append(files, engine.FeedbackFile{Name: h.Filename, ContentType: h.Header.Get("Content-Type"), Size: h.Size, Body: f})
	}
	id, err := s.E.SubmitFeedback(r.Context(), author, data, files)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]any{"id": id}})
}

// myFeedback is the author's own feedback, with the status and the response
// the developers gave it.
func (s *Server) myFeedback(w http.ResponseWriter, r *http.Request) {
	if !s.E.Cfg.Feedback.On() {
		http.NotFound(w, r)
		return
	}
	author, err := s.feedbackAuthor(r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.E.ListMyFeedback(r.Context(), author)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response{Data: list})
}
