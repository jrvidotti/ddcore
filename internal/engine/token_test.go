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
		t.Errorf("esperava 48 hex (192 bits), veio %d", len(token))
	}
	if time.Until(expires) < 50*time.Minute {
		t.Errorf("expiração muito curta: %v", expires)
	}

	// espiar não gasta
	if _, err := e.PeekToken(ctx, token); err != nil {
		t.Fatalf("PeekToken: %v", err)
	}
	if _, err := e.PeekToken(ctx, token); err != nil {
		t.Fatalf("espiar duas vezes não pode gastar: %v", err)
	}

	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		at, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		if err != nil {
			return err
		}
		if at.User != "ze@x.com" {
			t.Errorf("usuário errado: %q", at.User)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// gastar de novo é recusado, e com erro de validação e não 500
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		return err
	})
	if err == nil {
		t.Fatal("um token usado tem de ser recusado")
	}
	if cerr.From(err).Type != "ValidationError" {
		t.Errorf("esperava ValidationError, veio %s", cerr.From(err).Type)
	}
}

// O token é guardado como hash: quem lê o banco não consegue redefinir senha.
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
			t.Fatal("o token cru foi para o banco")
		}
		if len(h) != 64 {
			t.Errorf("esperava um sha256 hex, veio %d chars", len(h))
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
		t.Error("um token vencido não pode ser espiado")
	}
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := e.ConsumeToken(ctx, c.Tx, token, TokenReset)
		return err
	})
	if err == nil {
		t.Error("um token vencido não pode ser gasto")
	}
}

// Um token de convite não serve como token de recuperação e vice-versa: sem
// isso, um convite pendente seria um caminho para redefinir a senha de alguém.
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
		t.Error("um token de convite não pode ser gasto como recuperação")
	}
}

// A razão de ConsumeToken ser uma instrução só: dois envios do mesmo link não
// podem ambos vencer a corrida e ambos definir uma senha.
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
		t.Fatalf("exatamente um tem de vencer, venceram %d", wins)
	}
}

func TestSweepAuthRemovesOnlyWhatExpired(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()

	vivo, _, err := e.IssueToken(ctx, "ze@x.com", TokenReset, time.Hour, "", "")
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
		t.Errorf("esperava 1 token varrido, veio %d", n.Tokens)
	}
	if _, err := e.PeekToken(ctx, vivo); err != nil {
		t.Errorf("o token vivo não podia ter sido varrido: %v", err)
	}
}
