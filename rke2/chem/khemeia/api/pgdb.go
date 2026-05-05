package main

import "database/sql"

// DB is a type alias for sql.DB. MySQL uses ? placeholders natively.
type DB = sql.DB

// Tx is a type alias for sql.Tx.
type Tx = sql.Tx
