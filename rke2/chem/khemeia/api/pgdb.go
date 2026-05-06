// Package main provides thin wrapper types around *sql.DB and *sql.Tx that
// automatically rewrite MySQL-style positional placeholders ("?") to PostgreSQL
// positional parameters ("$1", "$2", …) before executing any statement.
//
// This lets the rest of the codebase use "?" consistently — a convention that
// reads cleanly — while the driver receives the correct "$N" syntax at runtime.
//
// Usage:
//
//	db := &DB{DB: sqlDB}           // wrap *sql.DB once at startup
//	tx := &Tx{Tx: sqlTx}          // wrap *sql.Tx inside BeginTx helpers
//
// All *DB methods mirror the *sql.DB API subset used in this codebase.
// All *Tx methods mirror the *sql.Tx API subset used in this codebase.
package main

import (
	"context"
	"database/sql"
	"strings"
)

// DB wraps *sql.DB and rewrites "?" placeholders to "$N" before execution.
type DB struct {
	*sql.DB
}

// Tx wraps *sql.Tx and rewrites "?" placeholders to "$N" before execution.
type Tx struct {
	*sql.Tx
}

// rebind converts a MySQL-style query using "?" placeholders to a PostgreSQL
// query using "$1", "$2", … positional parameters.
//
// It processes the query character by character, respecting single-quoted
// string literals (so that literal '?' inside strings is not replaced).
func rebind(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 16)

	n := 1
	inQuote := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case c == '\'' && !inQuote:
			inQuote = true
			b.WriteByte(c)
		case c == '\'' && inQuote:
			// Handle escaped single-quote ('').
			if i+1 < len(query) && query[i+1] == '\'' {
				b.WriteByte(c)
				b.WriteByte(c)
				i++
			} else {
				inQuote = false
				b.WriteByte(c)
			}
		case c == '?' && !inQuote:
			b.WriteByte('$')
			// Write decimal digits for n.
			if n < 10 {
				b.WriteByte(byte('0' + n))
			} else {
				// General path for n >= 10.
				digits := []byte{}
				tmp := n
				for tmp > 0 {
					digits = append([]byte{byte('0' + tmp%10)}, digits...)
					tmp /= 10
				}
				b.Write(digits)
			}
			n++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// --- DB methods ---

// ExecContext rewrites placeholders and calls the underlying ExecContext.
func (d *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.DB.ExecContext(ctx, rebind(query), args...)
}

// Exec rewrites placeholders and calls the underlying Exec.
func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.DB.Exec(rebind(query), args...)
}

// QueryContext rewrites placeholders and calls the underlying QueryContext.
func (d *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return d.DB.QueryContext(ctx, rebind(query), args...)
}

// Query rewrites placeholders and calls the underlying Query.
func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.DB.Query(rebind(query), args...)
}

// QueryRowContext rewrites placeholders and calls the underlying QueryRowContext.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return d.DB.QueryRowContext(ctx, rebind(query), args...)
}

// QueryRow rewrites placeholders and calls the underlying QueryRow.
func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.DB.QueryRow(rebind(query), args...)
}

// BeginTx begins a transaction and returns a wrapped *Tx.
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx}, nil
}

// PrepareContext rewrites placeholders and calls the underlying PrepareContext.
func (d *DB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return d.DB.PrepareContext(ctx, rebind(query))
}

// Prepare rewrites placeholders and calls the underlying Prepare.
func (d *DB) Prepare(query string) (*sql.Stmt, error) {
	return d.DB.Prepare(rebind(query))
}

// --- Tx methods ---

// ExecContext rewrites placeholders and calls the underlying ExecContext.
func (t *Tx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, rebind(query), args...)
}

// Exec rewrites placeholders and calls the underlying Exec.
func (t *Tx) Exec(query string, args ...interface{}) (sql.Result, error) {
	return t.Tx.Exec(rebind(query), args...)
}

// QueryContext rewrites placeholders and calls the underlying QueryContext.
func (t *Tx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, rebind(query), args...)
}

// Query rewrites placeholders and calls the underlying Query.
func (t *Tx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return t.Tx.Query(rebind(query), args...)
}

// QueryRowContext rewrites placeholders and calls the underlying QueryRowContext.
func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.Tx.QueryRowContext(ctx, rebind(query), args...)
}

// QueryRow rewrites placeholders and calls the underlying QueryRow.
func (t *Tx) QueryRow(query string, args ...interface{}) *sql.Row {
	return t.Tx.QueryRow(rebind(query), args...)
}

// PrepareContext rewrites placeholders and calls the underlying PrepareContext.
func (t *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.Tx.PrepareContext(ctx, rebind(query))
}

// Prepare rewrites placeholders and calls the underlying Prepare.
func (t *Tx) Prepare(query string) (*sql.Stmt, error) {
	return t.Tx.Prepare(rebind(query))
}
