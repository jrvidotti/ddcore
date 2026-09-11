package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

func TestTokenIsSingleUse(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()

	token, expires, err := e.IssueToken(ctx, "ze@x.com", TokenReset, time.Hour, "Administrator", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 48 {
		t.Errorf("expected 48 hex (192 bits), got %d", len(token))
	}
	if time.Until(expires) < 50*time.Minute {
		t.Errorf("expiration too short: %v", expires)
	}

	// peeking does not consume
	if _, err := e.PeekToken(ctx, token); err != nil {
		t.Fatalf("PeekToken: %v", err)
	}
	if _, err := e.PeekToken(ctx, token); err != nil {
		t.Fatalf("peeking twice must not consume: %v", err)
	}

	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		at, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		if err != nil {
			return err
		}
		if at.User != "ze@x.com" {
			t.Errorf("wrong user: %q", at.User)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// consuming again is rejected, and with ValidationError rather than 500
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		return err
	})
	if err == nil {
		t.Fatal("a used token must be rejected")
	}
	if cerr.From(err).Type != "ValidationError" {
		t.Errorf("expected ValidationError, got %s", cerr.From(err).Type)
	}
}

// The token is stored as a hash: someone reading the database cannot reset passwords.
func TestTokenIsStoredHashed(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()
	token, _, err := e.IssueToken(ctx, "ze@x.com", TokenReset, time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT token_hash FROM ddcore_auth_token`)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		h := db.Str(r["token_hash"])
		if strings.Contains(h, token) || h == token {
			t.Fatal("raw token was sent to the database")
		}
		if len(h) != 64 {
			t.Errorf("expected a sha256 hex, got %d chars", len(h))
		}
	}
}

func TestTokenExpires(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()
	token, _, err := e.IssueToken(ctx, "ze@x.com", TokenReset, -time.Minute, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.PeekToken(ctx, token); err == nil {
		t.Error("an expired token cannot be peeked")
	}
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		return err
	})
	if err == nil {
		t.Error("an expired token cannot be consumed")
	}
}

// An invite token cannot serve as a recovery token and vice-versa: without
// this, a pending invite would be a vector to reset someone's password.
func TestTokenKindIsChecked(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()
	token, _, err := e.IssueToken(ctx, "ze@x.com", TokenInvite, time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		return err
	})
	if err == nil {
		t.Error("an invite token cannot be consumed as recovery")
	}
}

// The reason ConsumeToken is a single statement: two submissions of the same link
// cannot both win the race and both set a password.
func TestTokenRaceHasExactlyOneWinner(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()
	token, _, err := e.IssueToken(ctx, "ze@x.com", TokenReset, time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}

	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer wg.Done()
			err := e.Run(ctx, "Administrator", func(c *Ctx) error {
				_, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
				return err
			})
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one must win, %d won", wins)
	}
}

func TestSweepAuthRemovesOnlyWhatExpired(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()

	active, _, err := e.IssueToken(ctx, "ze@x.com", TokenReset, time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.IssueToken(ctx, "ze@x.com", TokenInvite, -30*24*time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}

	n, err := e.SweepAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n.Tokens != 1 {
		t.Errorf("expected 1 swept token, got %d", n.Tokens)
	}
	if _, err := e.PeekToken(ctx, active); err != nil {
		t.Errorf("active token should not have been swept: %v", err)
	}
}
