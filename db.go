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
	`CREATE TABLE IF NOT EXISTS shares (
    server text not null,
    sharename text not null,
    state text,
    created_at datetime,
    PRIMARY KEY (server, sharename)
    )`,
}

func connectAndSetup(name string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", name+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	for _, stmt := range schema {
		_, err = db.Exec(stmt)
		if err != nil {
			return nil, err
		}
	}

	// db.SetMaxOpenConns(1)

	return db, nil
}

func addShare(db *sql.DB, servername, sharename string) error {
	sql := "INSERT INTO shares (server, sharename, state, created_at) VALUES ($1, $2, 'started', datetime('now'))"
	_, err := db.Exec(sql, servername, sharename)
	return err
}

func updateShare(db *sql.DB, servername, sharename, state string) error {
	sql := "UPDATE shares set state=$1 where server=$2 and sharename=$3"
	_, err := db.Exec(sql, state, servername, sharename)
	return err
}

func shareScanned(db *sql.DB, servername, sharename string) (bool, error) {
	var count int

	sql := "SELECT count(*) FROM shares where server=$1 and sharename=$2"
	if err := db.QueryRow(sql, servername, sharename).Scan(&count); err != nil {
		return false, err
	}

	return count > 0, nil
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
