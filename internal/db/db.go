// Package db wraps pgx with the query builder and schema migration.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the site's connections. On a site with tenancy there are two pools:
// Pool, whose connections are confined to the role row-level security holds,
// and Sys, the same login unconfined, for the framework's own work across
// tenants. Everything reaches for Pool; Sys is asked for by name, so that
// crossing spaces is always something somebody wrote down. Without tenancy
// they are one pool.
type DB struct {
	Pool *pgxpool.Pool
	Sys  *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*DB, error) { return OpenConfined(ctx, dsn, Options{}) }

// Options are what a site says about its connections beyond the DSN.
type Options struct {
	// TenantRole confines Pool on a site with tenancy; empty opens one pool.
	TenantRole string
	// MaxConns sizes Pool — ddcore.json's poolMaxConns. Zero leaves it to
	// the DSN's pool_max_conns, or to pgx's default. Sys keeps its own size:
	// it carries single statements, never a request.
	MaxConns int
}

// OpenConfined is Open for a site with tenancy: every connection of Pool
// sets its role to o.TenantRole as soon as the database has had tenancy
// applied, so a statement that names no tenant sees the platform space and
// nothing else. Confinement is a connection's resting state and crossing
// spaces is the explicit act — a statement somebody forgot to think about
// then finds no rows, instead of every tenant's.
//
// Before the first migration there is no role to take and no policy to be
// held by, and the connection stays what the DSN made it; Migrate resets the
// pool when it is done.
func OpenConfined(ctx context.Context, dsn string, o Options) (*DB, error) {
	tenantRole := o.TenantRole
	open := func(main, confined bool) (*pgxpool.Pool, error) {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return nil, err
		}
		if main && o.MaxConns > 0 {
			cfg.MaxConns = int32(o.MaxConns)
		}
		if confined {
			cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
				applied, err := TenancyApplied(ctx, c)
				if err != nil || !applied {
					return err
				}
				_, err = c.Exec(ctx, "SET ROLE "+Ident(tenantRole))
				// A database restored into a cluster that never had the role:
				// the marker came with the dump, the role did not. The next
				// migration creates it and resets the pool. Until then nothing
				// is lost — a document transaction sets the role itself and
				// fails on it, and a migration needs no role at all. SET ROLE
				// reports a missing role as 22023 (invalid_parameter_value);
				// 42704 (undefined_object) is kept in case a server says so.
				var pg *pgconn.PgError
				if errors.As(err, &pg) && (pg.Code == "22023" || pg.Code == "42704") {
					return nil
				}
				return err
			}
		} else if tenantRole != "" {
			// the system pool carries single statements, never a request
			cfg.MaxConns = 4
		}
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("postgres: %w", err)
		}
		return pool, nil
	}
	if tenantRole == "" {
		pool, err := open(true, false)
		if err != nil {
			return nil, err
		}
		return &DB{Pool: pool, Sys: pool}, nil
	}
	if !identRe.MatchString(tenantRole) {
		return nil, fmt.Errorf("invalid tenant role name %q", tenantRole)
	}
	pool, err := open(true, true)
	if err != nil {
		return nil, err
	}
	sys, err := open(false, false)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{Pool: pool, Sys: sys}, nil
}

func (d *DB) Close() {
	d.Pool.Close()
	if d.Sys != d.Pool {
		d.Sys.Close()
	}
}

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
