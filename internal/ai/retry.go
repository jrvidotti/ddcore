package ai

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// maxAttempts is how many times one call reaches the provider.
const maxAttempts = 3

// maxResponseBytes caps what is read of an answer: far above any real one,
// it only keeps a broken proxy from filling the memory.
const maxResponseBytes = 16 << 20

// backoff is the wait before the second and the third attempt.
var backoff = []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond}

// maxRetryAfter caps how long a Retry-After is honoured: a provider asking
// for a minute would rather be told the call failed.
const maxRetryAfter = 5 * time.Second

// transient is whether a status is worth another attempt.
func transient(status int) bool { return status == http.StatusTooManyRequests || status >= 500 }

// do sends the request build makes up to maxAttempts times within total,
// retrying on a transient status or a transport error. Each attempt gets the
// time that is left; a status that is not transient ends it at once, as does
// ctx. It returns the last status and body, a transient one included once the
// attempts or the time run out, for the caller to word.
func (c *Client) do(ctx context.Context, kind, model string, total time.Duration, build func(context.Context) (*http.Request, error)) (int, []byte, error) {
	deadline := time.Now().Add(total)
	for attempt := 1; ; attempt++ {
		if time.Until(deadline) <= 0 {
			return 0, nil, timedOut(total)
		}
		start := time.Now()
		status, body, retryAfter, err := c.once(ctx, deadline, build)
		c.Log.Info("ai call", "kind", kind, "model", model, "status", status,
			"latencyMs", time.Since(start).Milliseconds(), "attempt", attempt)
		if ctx.Err() != nil {
			return 0, nil, cerr.Unavailable("AI request cancelled")
		}
		if err == nil && !transient(status) {
			return status, body, nil
		}
		wait := backoff[min(attempt, len(backoff))-1]
		if retryAfter >= 0 {
			wait = min(retryAfter, maxRetryAfter)
		}
		if attempt == maxAttempts || wait >= time.Until(deadline) {
			if err == nil {
				return status, body, nil
			}
			if time.Until(deadline) <= 0 {
				return 0, nil, timedOut(total)
			}
			return 0, nil, cerr.Unavailable("AI provider could not be reached")
		}
		c.sleep(wait)
	}
}

// once makes one attempt with the deadline that is left. retryAfter is -1
// when the answer did not ask for a wait.
func (c *Client) once(ctx context.Context, deadline time.Time, build func(context.Context) (*http.Request, error)) (status int, body []byte, retryAfter time.Duration, err error) {
	retryAfter = -1
	actx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	req, err := build(actx)
	if err != nil {
		return 0, nil, retryAfter, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, retryAfter, err
	}
	defer resp.Body.Close()
	if body, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes)); err != nil {
		return 0, nil, retryAfter, err
	}
	if s, perr := strconv.Atoi(resp.Header.Get("Retry-After")); perr == nil && s >= 0 {
		retryAfter = time.Duration(s) * time.Second
	}
	return resp.StatusCode, body, retryAfter, nil
}

func timedOut(total time.Duration) error {
	return cerr.Unavailable("AI request timed out after {0}s", int(total.Round(time.Second).Seconds()))
}
