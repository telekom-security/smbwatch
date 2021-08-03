package main

import (
	"database/sql"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS files (
    server text,
    sharename text,

    name text,
    path text,
    extension text,

    size int,
    modified_at datetime,
    mode int)`,
	`CREATE INDEX IF NOT EXISTS files_extension ON files (extension)`,
	`CREATE INDEX IF NOT EXISTS files_name ON files (name)`,
}

func connectAndSetup(name string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", name)
	if err != nil {
		return nil, err
	}

	for _, stmt := range schema {
		_, err = db.Exec(stmt)
		if err != nil {
			return nil, err
		}
	}

	db.SetMaxOpenConns(1)

	return db, nil
}

func addFile(db *sql.DB, f ShareFile) error {

	ext := filepath.Ext(f.File.Name())
	if strings.HasPrefix(ext, ".") {
		ext = ext[1:]
	}

	sql := "INSERT INTO files (server, sharename, name, path, extension, size, modified_at, mode) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"
	_, err := db.Exec(
		sql,
		f.ServerName,
		f.ShareName,
		f.File.Name(),
		f.Folder,
		strings.ToLower(ext),
		f.File.Size(),
		f.File.ModTime(),
		f.File.Mode(),
	)

	return err
}
