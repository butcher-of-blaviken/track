package sqlite

import "io/fs"

func migrationsSub() (fs.FS, error) { return fs.Sub(migrationsFS, "migrations") }
