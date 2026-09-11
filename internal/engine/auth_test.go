package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
)

func TestValidatePassword(t *testing.T) {
	e := &Engine{
		Cfg: Config{
			Auth: config.AuthPolicy{
				MinPasswordLength: 8,
			},
		},
	}

	tests := []struct {
		name     string
		identity string
		password string
		wantErr  string
	}{
		{
			name:     "empty password",
			identity: "user@x.com",
			password: "",
			wantErr:  "The password must have at least 8 characters",
		},
		{
			name:     "too short whitespace",
			identity: "user@x.com",
			password: "   ",
			wantErr:  "The password must have at least 8 characters",
		},
		{
			name:     "blank whitespace meeting min length",
			identity: "user@x.com",
			password: "        ", // 8 spaces
			wantErr:  "The password cannot be blank",
		},
		{
			name:     "too short in ascii",
			identity: "user@x.com",
			password: "curta",
			wantErr:  "The password must have at least 8 characters",
		},
		{
			name:     "rune count vs byte count - 7 runes 8 bytes fails",
			identity: "user@x.com",
			password: "senhaç1", // 7 runes, 8 bytes in UTF-8
			wantErr:  "The password must have at least 8 characters",
		},
		{
			name:     "rune count vs byte count - 8 runes 9 bytes passes",
			identity: "user@x.com",
			password: "senhaç12", // 8 runes, 9 bytes in UTF-8
			wantErr:  "",
		},
		{
			name:     "exceeds max password length",
			identity: "user@x.com",
			password: strings.Repeat("a", maxPasswordLength+1),
			wantErr:  "The password must have at most 128 characters",
		},
		{
			name:     "exact max password length passes",
			identity: "user@x.com",
			password: strings.Repeat("a", maxPasswordLength),
			wantErr:  "",
		},
		{
			name:     "same as identity exact",
			identity: "ana@x.com",
			password: "ana@x.com",
			wantErr:  "The password cannot be the same as the username",
		},
		{
			name:     "same as identity case insensitive",
			identity: "ana@x.com",
			password: "ANA@X.COM",
			wantErr:  "The password cannot be the same as the username",
		},
		{
			name:     "same as identity with whitespace",
			identity: "  ana@x.com  ",
			password: "ana@x.com",
			wantErr:  "The password cannot be the same as the username",
		},
		{
			name:     "valid password",
			identity: "ana@x.com",
			password: "segredo123",
			wantErr:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := e.ValidatePassword(tt.identity, tt.password)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				cErr := cerr.From(err)
				if cErr.Type != "ValidationError" {
					t.Errorf("expected ValidationError, got %s", cErr.Type)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error %q to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestDropSessions(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()

	sid1 := RandomToken()
	sid2 := RandomToken()
	sid3 := RandomToken()

	// Cria duas sessões para ana e uma para bia
	for _, s := range []struct {
		sid  string
		user string
	}{
		{sid1, "ana@x.com"},
		{sid2, "ana@x.com"},
		{sid3, "bia@x.com"},
	} {
		if _, err := e.DB.Pool.Exec(ctx,
			`INSERT INTO ddcore_session (sid, "user", expires, ip, user_agent)
			 VALUES ($1, $2, now() + interval '1 day', '127.0.0.1', 'test')`,
			s.sid, s.user); err != nil {
			t.Fatal(err)
		}
		e.Cache.Set("sid:"+s.sid, s.user, time.Minute)
	}

	// DropSessions para ana poupando sid1
	n, err := e.DropSessions(ctx, e.DB.Pool, "ana@x.com", sid1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("esperava 1 sessão encerrada, veio %d", n)
	}

	// sid1 sobrevive
	if _, ok := e.Cache.Get("sid:" + sid1); !ok {
		t.Error("sid1 devia continuar no cache")
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT sid FROM ddcore_session WHERE sid = $1`, sid1)
	if err != nil || len(rows) != 1 {
		t.Error("sid1 devia continuar no banco")
	}

	// sid2 morreu no banco e no cache
	if _, ok := e.Cache.Get("sid:" + sid2); ok {
		t.Error("sid2 devia ter saído do cache")
	}
	rows, err = db.Select(ctx, e.DB.Pool, `SELECT sid FROM ddcore_session WHERE sid = $1`, sid2)
	if err != nil || len(rows) != 0 {
		t.Error("sid2 devia ter saído do banco")
	}

	// sid3 (bia) intacta
	if _, ok := e.Cache.Get("sid:" + sid3); !ok {
		t.Error("sid3 devia continuar no cache")
	}
	rows, err = db.Select(ctx, e.DB.Pool, `SELECT sid FROM ddcore_session WHERE sid = $1`, sid3)
	if err != nil || len(rows) != 1 {
		t.Error("sid3 devia continuar no banco")
	}

	// DropSessions sem poupar ninguém derruba sid1
	n, err = e.DropSessions(ctx, e.DB.Pool, "ana@x.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("esperava 1 sessão encerrada, veio %d", n)
	}
	if _, ok := e.Cache.Get("sid:" + sid1); ok {
		t.Error("sid1 devia ter saído do cache")
	}
	rows, err = db.Select(ctx, e.DB.Pool, `SELECT sid FROM ddcore_session WHERE sid = $1`, sid1)
	if err != nil || len(rows) != 0 {
		t.Error("sid1 devia ter saído do banco")
	}
}
