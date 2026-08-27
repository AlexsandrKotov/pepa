// Package migrations embeds all SQL migration files for use by the
// database migration runner. This ensures the binary carries its own
// migrations — no need to mount SQL files at deploy time.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
