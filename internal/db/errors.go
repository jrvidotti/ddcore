package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolation is Postgres's SQLSTATE for a unique or primary-key index a
// write collided with.
const uniqueViolation = "23505"

// UniqueViolation reports the index a write collided with, when it collided
// with one. The index name is the whole point: it is what tells a business key
// apart from a single unique field and from the primary key, and so what turns
// "duplicate key value violates unique constraint" into a sentence naming the
// value the reader has to change.
//
// It is the guard the pre-checks cannot be. A SELECT before the write cannot
// see a row another transaction has not committed yet, so two transactions can
// both pass validation and still be one document apart — and only the index
// refuses the second one.
func UniqueViolation(err error) (index string, ok bool) {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != uniqueViolation {
		return "", false
	}
	return pg.ConstraintName, true
}

// UndefinedTable reports whether err is Postgres saying a relation does not
// exist — a ledger read on a database no migration has touched yet.
func UndefinedTable(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42P01"
}

// UndefinedColumn reports whether err is Postgres saying a column does not
// exist — a query written for the current schema run against a database an
// older binary left behind.
func UndefinedColumn(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42703"
}
