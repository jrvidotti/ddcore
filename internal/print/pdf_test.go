package print

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

type mockRenderer struct {
	delay      time.Duration
	concurrent int32
	maxSeen    int32
}

func (m *mockRenderer) RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error) {
	cur := atomic.AddInt32(&m.concurrent, 1)
	defer atomic.AddInt32(&m.concurrent, -1)

	// Track peak concurrent executions
	for {
		seen := atomic.LoadInt32(&m.maxSeen)
		if cur <= seen || atomic.CompareAndSwapInt32(&m.maxSeen, seen, cur) {
			break
		}
	}

	select {
	case <-time.After(m.delay):
		return []byte("%PDF-1.4 mock"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestLimiterRenderer(t *testing.T) {
	mock := &mockRenderer{delay: 50 * time.Millisecond}
	limiter := NewLimiterRenderer(mock, 2)

	var wg sync.WaitGroup
	ctx := context.Background()

	// Launch 5 concurrent calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := limiter.RenderPDF(ctx, "<h1>Test</h1>", PDFOptions{})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	if mock.maxSeen > 2 {
		t.Fatalf("expected max concurrent to be at most 2, got %d", mock.maxSeen)
	}

	// Test context cancellation while waiting for semaphore
	slowMock := &mockRenderer{delay: 200 * time.Millisecond}
	slowLimiter := NewLimiterRenderer(slowMock, 1)

	// Fill the 1 slot
	go slowLimiter.RenderPDF(ctx, "<h1>First</h1>", PDFOptions{})
	time.Sleep(10 * time.Millisecond)

	// Next call with fast timeout should abort
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	_, err := slowLimiter.RenderPDF(timeoutCtx, "<h1>Second</h1>", PDFOptions{})
	if err == nil {
		t.Fatal("expected timeout error when acquiring limiter semaphore, got nil")
	}
}

func TestGotenbergRenderer(t *testing.T) {
	var receivedHTML string
	var receivedLandscape string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/chromium/convert/html" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		receivedLandscape = r.FormValue("landscape")
		file, _, err := r.FormFile("files")
		if err == nil {
			defer file.Close()
			b, _ := io.ReadAll(file)
			receivedHTML = string(b)
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("%PDF-1.4 Gotenberg output"))
	}))
	defer ts.Close()

	renderer := NewGotenbergRenderer(ts.URL)
	pdf, err := renderer.RenderPDF(context.Background(), "<h1>Hello Gotenberg</h1>", PDFOptions{Landscape: true, Format: "A4"})
	if err != nil {
		t.Fatalf("unexpected Gotenberg error: %v", err)
	}

	if string(pdf) != "%PDF-1.4 Gotenberg output" {
		t.Fatalf("unexpected PDF output: %s", string(pdf))
	}
	if !strings.Contains(receivedHTML, "<h1>Hello Gotenberg</h1>") {
		t.Fatalf("HTML content not sent to Gotenberg: %s", receivedHTML)
	}
	if receivedLandscape != "true" {
		t.Fatalf("expected landscape flag 'true', got: %s", receivedLandscape)
	}
}

func TestCommandRenderer(t *testing.T) {
	renderer := &CommandRenderer{
		CommandTemplate: `echo '%PDF-1.4 custom' > {out}`,
	}

	pdf, err := renderer.RenderPDF(context.Background(), "<h1>Custom</h1>", PDFOptions{})
	if err != nil {
		t.Fatalf("unexpected CommandRenderer error: %v", err)
	}

	if !strings.Contains(string(pdf), "%PDF-1.4 custom") {
		t.Fatalf("unexpected PDF output: %s", string(pdf))
	}
}

func TestUnavailableRenderer(t *testing.T) {
	renderer := UnavailableRenderer{}
	_, err := renderer.RenderPDF(context.Background(), "<h1>Test</h1>", PDFOptions{})
	if err == nil {
		t.Fatal("expected error from UnavailableRenderer, got nil")
	}
	if cerr.From(err).Status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 status, got: %d", cerr.From(err).Status)
	}
}

func TestGetDefaultRenderer_EnvDetection(t *testing.T) {
	ResetDefaultRenderer()
	defer ResetDefaultRenderer()

	// 1. Gotenberg via ENV
	os.Setenv("DDCORE_GOTENBERG_URL", "http://localhost:3000")
	r := GetDefaultRenderer()
	limiter, ok := r.(*LimiterRenderer)
	if !ok {
		t.Fatalf("expected LimiterRenderer, got: %T", r)
	}
	if _, ok := limiter.inner.(*GotenbergRenderer); !ok {
		t.Fatalf("expected inner to be *GotenbergRenderer, got: %T", limiter.inner)
	}

	// 2. Command via ENV
	os.Unsetenv("DDCORE_GOTENBERG_URL")
	os.Setenv("DDCORE_PDF_COMMAND", "sh -c echo {out}")
	ResetDefaultRenderer()

	r2 := GetDefaultRenderer()
	limiter2, ok := r2.(*LimiterRenderer)
	if !ok {
		t.Fatalf("expected LimiterRenderer, got: %T", r2)
	}
	if _, ok := limiter2.inner.(*CommandRenderer); !ok {
		t.Fatalf("expected inner to be *CommandRenderer, got: %T", limiter2.inner)
	}

	os.Unsetenv("DDCORE_PDF_COMMAND")
	ResetDefaultRenderer()
}
