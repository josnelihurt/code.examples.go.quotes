// Package migrations exposes the SQL migration files as an embed.FS so the
// infrastructure layer can run them from source (iofs) without shipping a
// migrate CLI. The embed directive must live beside the files it embeds — Go
// cannot embed across parent directories — which is why this file lives in
// migrations/ rather than next to the Migrate helper.
package migrations

import "embed"

// FS holds every golang-migrate pair (*.up.sql / *.down.sql) in this
// directory; iofs.New(FS, ".") is the migration source.
//
//go:embed *.sql
var FS embed.FS
