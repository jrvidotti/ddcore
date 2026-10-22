package engine

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// Rotating the master key (#134). A server given DDCORE_SECRET_KEY_PREVIOUS
// reads with either key and writes with the current one; `ddcore vault rekey`
// then moves what is still under the previous key, and the census below is
// how doctor tells when nothing is left and the previous key can go.
//
// Both work on the pool row-level security does not hold, across every space:
// a rotation is the site's, not a tenant's.

// VaultKeyCensus counts the secrets by the key that opens them. Unreadable
// names those that open under neither — with their tenant in front on a site
// with tenancy — and never holds a value.
type VaultKeyCensus struct {
	PreviousConfigured bool     `json:"previousConfigured"`
	Current            int      `json:"current"`
	Previous           int      `json:"previous"`
	Unreadable         []string `json:"unreadable"`
}

// VaultRekeyOpts are the knobs of a re-key. BatchSize is how many secrets
// each transaction moves; zero is 100.
type VaultRekeyOpts struct {
	DryRun    bool
	BatchSize int
	Actor     string
}

// VaultRekeyResult is what a re-key did, and the census after it.
type VaultRekeyResult struct {
	DryRun bool           `json:"dryRun"`
	Moved  int            `json:"moved"`
	Census VaultKeyCensus `json:"census"`
}

type vaultRow struct {
	tenant, name, keyID string
	ciphertext, nonce   []byte
}

// label is how a secret is named in a report: its name, after its tenant's
// slug when it has one.
func (r vaultRow) label() string {
	if r.tenant != "" {
		return r.tenant + "/" + r.name
	}
	return r.name
}

// selectVaultRows reads secrets with whatever identifies them. tail is the
// rest of the statement after FROM ddcore_vault.
func selectVaultRows(ctx context.Context, q db.Querier, tenancy bool, tail string, args ...any) ([]vaultRow, error) {
	tenant := "''"
	if tenancy {
		tenant = "tenant"
	}
	rows, err := q.Query(ctx, `SELECT `+tenant+`, name, coalesce(key_id, ''), ciphertext, nonce FROM ddcore_vault `+tail, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (vaultRow, error) {
		var r vaultRow
		err := row.Scan(&r.tenant, &r.name, &r.keyID, &r.ciphertext, &r.nonce)
		return r, err
	})
}

// VaultCensus opens every secret in memory, discards what it reads, and counts
// them by the key that opened each.
func (e *Engine) VaultCensus(ctx context.Context) (*VaultKeyCensus, error) {
	kr, err := VaultKeys()
	if err != nil {
		return nil, err
	}
	if err := vaultMigrated(ctx, e.DB.Sys); err != nil {
		return nil, err
	}
	on, err := e.tenancy(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := selectVaultRows(ctx, e.DB.Sys, on, `ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	census := &VaultKeyCensus{PreviousConfigured: kr.previous != nil, Unreadable: []string{}}
	for _, r := range rows {
		_, used, err := kr.open(r.keyID, r.ciphertext, r.nonce)
		switch {
		case err != nil:
			census.Unreadable = append(census.Unreadable, r.label())
		case used.id == kr.current.id:
			census.Current++
		default:
			census.Previous++
		}
	}
	return census, nil
}

// VaultRekey re-encrypts under the current key every secret not yet
// fingerprinted with it, a batch per transaction. A secret that opens under
// neither key stops it, named but never shown; the batches already committed
// stay moved. A run that is not a dry run records one Audit Event, of counts.
func (e *Engine) VaultRekey(ctx context.Context, o VaultRekeyOpts) (*VaultRekeyResult, error) {
	kr, err := VaultKeys()
	if err != nil {
		return nil, err
	}
	if err := vaultMigrated(ctx, e.DB.Sys); err != nil {
		return nil, err
	}
	on, err := e.tenancy(ctx)
	if err != nil {
		return nil, err
	}
	res := &VaultRekeyResult{DryRun: o.DryRun}
	if !o.DryRun {
		size := o.BatchSize
		if size <= 0 {
			size = 100
		}
		var runErr error
		for {
			moved, done, err := e.rekeyBatch(ctx, kr, on, size)
			if err != nil {
				runErr = err
				break
			}
			res.Moved += moved
			if done {
				break
			}
		}
		census, err := e.VaultCensus(ctx)
		if err == nil {
			res.Census = *census
		}
		detail := map[string]any{"moved": res.Moved, "current": res.Census.Current,
			"previous": res.Census.Previous, "unreadable": len(res.Census.Unreadable)}
		if runErr != nil {
			detail["stopped"] = true
		}
		if err := e.RecordAudit(ctx, o.Actor, "vault.rekey", "Allowed", "", "", detail); err != nil {
			e.Log.Warn("could not audit the vault re-key", "err", db.RedactError(err))
		}
		if runErr != nil {
			return res, runErr
		}
		return res, err
	}
	census, err := e.VaultCensus(ctx)
	if err != nil {
		return nil, err
	}
	res.Census = *census
	return res, nil
}

// rekeyBatch moves up to size secrets in one transaction. done is true once
// there is nothing left to move.
func (e *Engine) rekeyBatch(ctx context.Context, kr vaultKeyring, tenancy bool, size int) (moved int, done bool, err error) {
	tx, err := e.DB.Sys.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)
	order := "name"
	where := "name = $4"
	if tenancy {
		order = "tenant, name"
		where = "tenant = $5 AND name = $4"
	}
	rows, err := selectVaultRows(ctx, tx, tenancy,
		`WHERE key_id IS DISTINCT FROM $1 ORDER BY `+order+` LIMIT $2 FOR UPDATE`, kr.current.id, size)
	if err != nil {
		return 0, false, err
	}
	for _, r := range rows {
		plaintext, _, err := kr.open(r.keyID, r.ciphertext, r.nonce)
		if err != nil {
			return 0, false, cerr.Validation("Vault secret {0} opens under neither {1} nor {2}; delete or set it again, or provide the key it was written with",
				r.label(), MasterKeyEnvName, PreviousKeyEnvName)
		}
		ciphertext, nonce, err := EncryptVault(kr.current.key, plaintext)
		if err != nil {
			return 0, false, err
		}
		args := []any{ciphertext, nonce, kr.current.id, r.name}
		if tenancy {
			args = append(args, r.tenant)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE ddcore_vault SET ciphertext = $1, nonce = $2, key_id = $3, updated = now() WHERE %s`, where), args...); err != nil {
			return 0, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return len(rows), len(rows) < size, nil
}
