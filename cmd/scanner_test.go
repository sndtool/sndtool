package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	id3 "github.com/bogem/id3v2/v2"
)

// createTestMP3 writes a minimal valid MP3 file with the given ID3 tags.
func createTestMP3(t *testing.T, path, artist, album, title, year string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	// Write a minimal MP3 frame (0xFF 0xFB 0x90 0x00 + padding to 417 bytes).
	frame := make([]byte, 417)
	frame[0] = 0xFF
	frame[1] = 0xFB
	frame[2] = 0x90
	frame[3] = 0x00

	if err := os.WriteFile(path, frame, 0o644); err != nil {
		t.Fatalf("write mp3 %s: %v", path, err)
	}

	tag, err := id3.Open(path, id3.Options{Parse: true})
	if err != nil {
		t.Fatalf("id3 open %s: %v", path, err)
	}
	defer tag.Close()

	tag.SetArtist(artist)
	tag.SetAlbum(album)
	tag.SetTitle(title)
	tag.SetYear(year)

	if err := tag.Save(); err != nil {
		t.Fatalf("id3 save %s: %v", path, err)
	}
}

func TestScanDir_InitialScan(t *testing.T) {
	dir := t.TempDir()
	db := mustOpenDB(t)

	createTestMP3(t, filepath.Join(dir, "a.mp3"), "Artist A", "Album A", "Track A", "2020")
	createTestMP3(t, filepath.Join(dir, "b.mp3"), "Artist B", "Album B", "Track B", "2021")
	createTestMP3(t, filepath.Join(dir, "sub", "c.mp3"), "Artist C", "Album C", "Track C", "2022")

	// Create a non-mp3 file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	stats, err := ScanDir(db, dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}

	if stats.Added != 3 {
		t.Errorf("Added = %d, want 3", stats.Added)
	}
	if stats.Updated != 0 {
		t.Errorf("Updated = %d, want 0", stats.Updated)
	}
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", stats.Deleted)
	}

	tracks, err := QueryTracks(db, nil, nil)
	if err != nil {
		t.Fatalf("QueryTracks: %v", err)
	}
	if len(tracks) != 3 {
		t.Errorf("QueryTracks returned %d tracks, want 3", len(tracks))
	}
}

func TestScanDir_IncrementalUpdate(t *testing.T) {
	dir := t.TempDir()
	db := mustOpenDB(t)

	path := filepath.Join(dir, "track.mp3")
	createTestMP3(t, path, "Old Artist", "Old Album", "Old Title", "2000")

	if _, err := ScanDir(db, dir); err != nil {
		t.Fatalf("first ScanDir: %v", err)
	}

	// Recreate the file with different tags and bump mtime by 2 seconds to
	// ensure the scanner detects the change even on coarse-grained filesystems.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	createTestMP3(t, path, "New Artist", "New Album", "New Title", "2024")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	stats, err := ScanDir(db, dir)
	if err != nil {
		t.Fatalf("second ScanDir: %v", err)
	}

	if stats.Updated != 1 {
		t.Errorf("Updated = %d, want 1", stats.Updated)
	}
	if stats.Added != 0 {
		t.Errorf("Added = %d, want 0", stats.Added)
	}

	tracks, err := QueryTracks(db, nil, nil)
	if err != nil {
		t.Fatalf("QueryTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("QueryTracks returned %d tracks, want 1", len(tracks))
	}
	if tracks[0].Artist != "New Artist" {
		t.Errorf("Artist = %q, want %q", tracks[0].Artist, "New Artist")
	}
	if tracks[0].Title != "New Title" {
		t.Errorf("Title = %q, want %q", tracks[0].Title, "New Title")
	}
}

func TestScanDir_DeletedFiles(t *testing.T) {
	dir := t.TempDir()
	db := mustOpenDB(t)

	path := filepath.Join(dir, "gone.mp3")
	createTestMP3(t, path, "Artist", "Album", "Title", "2020")

	if _, err := ScanDir(db, dir); err != nil {
		t.Fatalf("first ScanDir: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	stats, err := ScanDir(db, dir)
	if err != nil {
		t.Fatalf("second ScanDir: %v", err)
	}

	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}

	tracks, err := QueryTracks(db, nil, nil)
	if err != nil {
		t.Fatalf("QueryTracks: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("QueryTracks returned %d tracks, want 0", len(tracks))
	}
}
