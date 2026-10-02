// Package db wraps pgx with the query builder and schema migration.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*DB, error) { return OpenConfined(ctx, dsn, "") }

// OpenConfined is Open for a site with tenancy: every connection sets its
// role to tenantRole as soon as the database has had tenancy applied, so a
// statement that names no tenant sees the platform space and nothing else.
// Confinement is the connection's resting state and crossing spaces is the
// explicit act (Elevate) — a statement somebody forgot to think about then
// finds no rows, instead of every tenant's.
//
// Before the first migration there is no role to take and no policy to be
// held by, and the connection stays what the DSN made it; Migrate resets the
// pool when it is done.
func OpenConfined(ctx context.Context, dsn, tenantRole string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if tenantRole != "" {
		if !identRe.MatchString(tenantRole) {
			return nil, fmt.Errorf("invalid tenant role name %q", tenantRole)
		}
		cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
			applied, err := TenancyApplied(ctx, c)
			if err != nil || !applied {
				return err
			}
			_, err = c.Exec(ctx, "SET ROLE "+Ident(tenantRole))
			return err
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) Close() { d.Pool.Close() }

// Querier is satisfied by pgx.Tx and *pgxpool.Pool.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Rows reads all rows as maps keyed by column name.
func Rows(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	descs := rows.FieldDescriptions()
	var out []map[string]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, len(descs))
		for i, d := range descs {
			m[string(d.Name)] = NormalizeOID(vals[i], d.DataTypeOID)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Select runs a query and returns rows as maps.
func Select(ctx context.Context, q Querier, sql string, args ...any) ([]map[string]any, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%w\nSQL: %s", err, sql)
	}
	return Rows(rows)
}
