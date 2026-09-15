package print

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// PDFOptions controls the output page layout for PDF generation.
type PDFOptions struct {
	Format       string // "A4", "Letter", etc. Default "A4"
	Landscape    bool
	MarginTop    string // e.g. "0mm", "10mm"
	MarginBottom string
	MarginLeft   string
	MarginRight  string
}

// PDFRenderer converts an HTML document string into raw PDF bytes.
type PDFRenderer interface {
	RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error)
}

// LimiterRenderer wraps another PDFRenderer with a concurrency semaphore
// to prevent memory exhaustion during concurrent PDF generation requests.
type LimiterRenderer struct {
	inner PDFRenderer
	sem   chan struct{}
}

// NewLimiterRenderer creates a concurrency-limited PDF renderer.
func NewLimiterRenderer(inner PDFRenderer, maxConcurrent int) *LimiterRenderer {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &LimiterRenderer{
		inner: inner,
		sem:   make(chan struct{}, maxConcurrent),
	}
}

// RenderPDF acquires a semaphore slot before calling the inner renderer.
func (r *LimiterRenderer) RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error) {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.inner.RenderPDF(ctx, htmlContent, opts)
}

// GotenbergRenderer converts HTML to PDF via a remote Gotenberg HTTP service.
type GotenbergRenderer struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewGotenbergRenderer creates a new Gotenberg-backed PDF renderer.
func NewGotenbergRenderer(baseURL string) *GotenbergRenderer {
	baseURL = strings.TrimRight(baseURL, "/")
	return &GotenbergRenderer{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// RenderPDF sends a multipart POST to Gotenberg's Chromium HTML convert endpoint.
func (g *GotenbergRenderer) RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Attach index.html
	part, err := writer.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, cerr.Internal("failed to prepare Gotenberg payload: {0}", err.Error())
	}
	if _, err := io.WriteString(part, htmlContent); err != nil {
		return nil, cerr.Internal("failed to write Gotenberg HTML payload: {0}", err.Error())
	}

	// Add options
	if opts.Landscape {
		writer.WriteField("landscape", "true")
	}
	if opts.Format != "" {
		switch strings.ToUpper(opts.Format) {
		case "LETTER":
			writer.WriteField("paperWidth", "8.5")
			writer.WriteField("paperHeight", "11")
		case "A4":
			writer.WriteField("paperWidth", "8.27")
			writer.WriteField("paperHeight", "11.7")
		}
	}
	if opts.MarginTop != "" {
		writer.WriteField("marginTop", opts.MarginTop)
	}
	if opts.MarginBottom != "" {
		writer.WriteField("marginBottom", opts.MarginBottom)
	}
	if opts.MarginLeft != "" {
		writer.WriteField("marginLeft", opts.MarginLeft)
	}
	if opts.MarginRight != "" {
		writer.WriteField("marginRight", opts.MarginRight)
	}

	if err := writer.Close(); err != nil {
		return nil, cerr.Internal("failed to finalize Gotenberg payload: {0}", err.Error())
	}

	endpoint := g.BaseURL + "/forms/chromium/convert/html"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, cerr.Internal("failed to build Gotenberg request: {0}", err.Error())
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return nil, cerr.Unavailable("failed to connect to Gotenberg service at {0}: {1}", g.BaseURL, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, cerr.Internal("Gotenberg PDF generation failed (status {0}): {1}", resp.StatusCode, string(respBytes))
	}

	pdfBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, cerr.Internal("failed to read Gotenberg PDF response: {0}", err.Error())
	}

	return pdfBytes, nil
}

// ChromeRenderer renders HTML to PDF using a local headless Chromium or Google Chrome binary.
type ChromeRenderer struct {
	ChromePath string
}

// RenderPDF executes the chrome executable with --headless and --print-to-pdf.
func (c *ChromeRenderer) RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "ddcore_print_*")
	if err != nil {
		return nil, cerr.Internal("failed to create temp directory for PDF render: {0}", err.Error())
	}
	defer os.RemoveAll(tmpDir)

	htmlPath := filepath.Join(tmpDir, "index.html")
	pdfPath := filepath.Join(tmpDir, "output.pdf")

	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0o600); err != nil {
		return nil, cerr.Internal("failed to write temporary HTML file: {0}", err.Error())
	}

	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--no-pdf-header-footer",
		fmt.Sprintf("--print-to-pdf=%s", pdfPath),
		htmlPath,
	}

	cmd := exec.CommandContext(ctx, c.ChromePath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try fallback with classic --headless if --headless=new failed
		args[0] = "--headless"
		cmdFallback := exec.CommandContext(ctx, c.ChromePath, args...)
		outputFallback, errFallback := cmdFallback.CombinedOutput()
		if errFallback != nil {
			return nil, cerr.Internal("Chromium PDF conversion failed: {0}, output: {1}", errFallback.Error(), string(outputFallback)+string(output))
		}
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, cerr.Internal("failed to read rendered PDF from {0}: {1}", pdfPath, err.Error())
	}

	return pdfBytes, nil
}

// CommandRenderer renders HTML to PDF using a custom CLI command template.
// Placeholders `{in}` and `{out}` are replaced with the input HTML and output PDF paths.
type CommandRenderer struct {
	CommandTemplate string
}

// RenderPDF executes the command template with replaced `{in}` and `{out}` arguments.
func (cmdR *CommandRenderer) RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "ddcore_print_cmd_*")
	if err != nil {
		return nil, cerr.Internal("failed to create temp directory for PDF render: {0}", err.Error())
	}
	defer os.RemoveAll(tmpDir)

	htmlPath := filepath.Join(tmpDir, "index.html")
	pdfPath := filepath.Join(tmpDir, "output.pdf")

	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0o600); err != nil {
		return nil, cerr.Internal("failed to write temporary HTML file: {0}", err.Error())
	}

	cmdStr := cmdR.CommandTemplate
	cmdStr = strings.ReplaceAll(cmdStr, "{in}", htmlPath)
	cmdStr = strings.ReplaceAll(cmdStr, "{out}", pdfPath)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, cerr.Internal("PDF command failed: {0}, output: {1}", err.Error(), string(output))
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, cerr.Internal("failed to read rendered PDF: {0}", err.Error())
	}

	return pdfBytes, nil
}

// UnavailableRenderer is returned when no PDF renderer is available.
type UnavailableRenderer struct{}

// RenderPDF returns an HTTP 503 Service Unavailable error explaining browser print fallback.
func (UnavailableRenderer) RenderPDF(ctx context.Context, htmlContent string, opts PDFOptions) ([]byte, error) {
	return nil, cerr.Unavailable("No PDF renderer is configured or installed on the server. Please print directly using your browser.")
}

var (
	defaultRendererLock sync.Mutex
	defaultRenderer     PDFRenderer
)

// FindChrome searches the system for an installed Chromium / Google Chrome binary.
func FindChrome() string {
	var candidates []string

	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"google-chrome",
			"chromium",
			"chromium-browser",
		}
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			"chrome.exe",
		}
	default: // Linux / BSD
		candidates = []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"headless-shell",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}

	for _, c := range candidates {
		if filepath.IsAbs(c) {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c
			}
		} else {
			if path, err := exec.LookPath(c); err == nil {
				return path
			}
		}
	}

	return ""
}

// GetDefaultRenderer detects and returns the best available PDFRenderer configured
// for this system, wrapped in a concurrency limiter.
func GetDefaultRenderer() PDFRenderer {
	defaultRendererLock.Lock()
	defer defaultRendererLock.Unlock()

	if defaultRenderer != nil {
		return defaultRenderer
	}

	// 1. Environment variable: Gotenberg URL
	if gotenbergURL := os.Getenv("DDCORE_GOTENBERG_URL"); gotenbergURL != "" {
		defaultRenderer = NewLimiterRenderer(NewGotenbergRenderer(gotenbergURL), 5)
		return defaultRenderer
	}

	// 2. Environment variable: Custom PDF command
	if pdfCmd := os.Getenv("DDCORE_PDF_COMMAND"); pdfCmd != "" {
		defaultRenderer = NewLimiterRenderer(&CommandRenderer{CommandTemplate: pdfCmd}, 3)
		return defaultRenderer
	}

	// 3. Local Chromium / Chrome discovery
	if chromePath := FindChrome(); chromePath != "" {
		defaultRenderer = NewLimiterRenderer(&ChromeRenderer{ChromePath: chromePath}, 3)
		return defaultRenderer
	}

	// 4. Graceful fallback
	defaultRenderer = UnavailableRenderer{}
	return defaultRenderer
}

// ResetDefaultRenderer allows tests to clear cached renderer.
func ResetDefaultRenderer() {
	defaultRendererLock.Lock()
	defer defaultRendererLock.Unlock()
	defaultRenderer = nil
}
