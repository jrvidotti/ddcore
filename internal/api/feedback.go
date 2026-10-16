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
	data, files, done, err := s.feedbackUpload(w, r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	defer done()
	author, err := s.feedbackAuthor(r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	id, err := s.E.SubmitFeedback(r.Context(), author, data, files)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]any{"id": id}})
}

// feedbackUpload reads the dialog's multipart body: `data` as a JSON object
// and up to engine.FeedbackMaxFiles `files`, together under the upload limit.
// done closes the files and removes their temporary copies.
func (s *Server) feedbackUpload(w http.ResponseWriter, r *http.Request) (map[string]any, []engine.FeedbackFile, func(), error) {
	max := s.MaxUpload
	if max <= 0 {
		max = engine.DefaultMaxUpload
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, nil, cerr.Validation("Invalid upload: {0}", err)
	}
	var opened []multipart.File
	done := func() {
		for _, f := range opened {
			f.Close()
		}
		r.MultipartForm.RemoveAll()
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(r.FormValue("data")), &data); err != nil || data == nil {
		done()
		return nil, nil, nil, cerr.Validation("Invalid {0}: {1}", "data", "expected a JSON object")
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) > engine.FeedbackMaxFiles {
		done()
		return nil, nil, nil, cerr.Validation("At most {0} files can be attached", engine.FeedbackMaxFiles).WithTitleKey("Too many files")
	}
	files := make([]engine.FeedbackFile, 0, len(headers))
	for _, h := range headers {
		f, err := h.Open()
		if err != nil {
			done()
			return nil, nil, nil, cerr.Validation("Invalid upload: {0}", err)
		}
		opened = append(opened, f)
		files = append(files, engine.FeedbackFile{Name: h.Filename, ContentType: h.Header.Get("Content-Type"), Size: h.Size, Body: f})
	}
	return data, files, done, nil
}

// assistFeedback is the dialog's "Fill in with AI": the same body as a
// submission, with the type and whatever the user typed so far, and the
// fields filled in from the audio notes, the screenshots and the page as the
// answer. Who asks and what the site and the page are is read in one short
// unit of work; the provider calls that follow hold no transaction.
func (s *Server) assistFeedback(w http.ResponseWriter, r *http.Request) {
	if !s.E.Cfg.Feedback.AssistOn(s.E.Cfg.AI) {
		http.NotFound(w, r)
		return
	}
	data, files, done, err := s.feedbackUpload(w, r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	defer done()
	values, pageURL, context, transcripts, err := engine.FeedbackAssistValues(data)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	in := engine.FeedbackAssistInput{Values: values, PageURL: pageURL, Context: context, Transcripts: transcripts, Files: files}
	routeDoctype := ""
	if m, ok := data["context"].(map[string]any); ok {
		routeDoctype, _ = m["doctype"].(string)
	}
	_, _, err = s.execute(r, nil, func(c *engine.Ctx) (any, error) {
		a, err := c.FeedbackAuthorOf()
		if err != nil {
			return nil, err
		}
		in.Author, in.Lang, in.Site = a, c.Lang, c.FeedbackSiteInfo(routeDoctype, pageURL)
		return nil, nil
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	res, err := s.E.AssistFeedback(r.Context(), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response{Data: res})
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
