// Package embeddb provides the embedded SQLite database.
package embeddb

import _ "embed"

//go:embed api.db
var DB []byte
