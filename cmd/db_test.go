package main

import (
	"database/sql"
	"testing"
)

func TestOpenDB_CreatesSchema(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	tables := []string{"tracks", "playlists", "playlist_tracks"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("table %q not found", table)
		} else if err != nil {
			t.Errorf("querying table %q: %v", table, err)
		}
	}
}
