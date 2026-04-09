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

// --- helpers ---

func mustOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func sampleTrack(path string) TrackRecord {
	return TrackRecord{
		Path:     path,
		Artist:   "Test Artist",
		Album:    "Test Album",
		Title:    "Test Title",
		Year:     "2025",
		Genre:    "Rock",
		Duration: 180.5,
		Mtime:    1000,
	}
}

// --- Task 2: Track CRUD ---

func TestUpsertTrack(t *testing.T) {
	db := mustOpenDB(t)

	tr := sampleTrack("/music/track1.mp3")
	if err := UpsertTrack(db, tr); err != nil {
		t.Fatalf("UpsertTrack insert: %v", err)
	}

	got, err := GetTrack(db, tr.Path)
	if err != nil {
		t.Fatalf("GetTrack: %v", err)
	}
	if got.Artist != tr.Artist {
		t.Errorf("Artist: got %q want %q", got.Artist, tr.Artist)
	}
	if got.Album != tr.Album {
		t.Errorf("Album: got %q want %q", got.Album, tr.Album)
	}
	if got.Title != tr.Title {
		t.Errorf("Title: got %q want %q", got.Title, tr.Title)
	}
	if got.Duration != tr.Duration {
		t.Errorf("Duration: got %v want %v", got.Duration, tr.Duration)
	}
	if got.Mtime != tr.Mtime {
		t.Errorf("Mtime: got %v want %v", got.Mtime, tr.Mtime)
	}

	// update
	tr.Artist = "Updated Artist"
	tr.Mtime = 2000
	if err := UpsertTrack(db, tr); err != nil {
		t.Fatalf("UpsertTrack update: %v", err)
	}
	got, err = GetTrack(db, tr.Path)
	if err != nil {
		t.Fatalf("GetTrack after update: %v", err)
	}
	if got.Artist != "Updated Artist" {
		t.Errorf("Artist after update: got %q want %q", got.Artist, "Updated Artist")
	}
	if got.Mtime != 2000 {
		t.Errorf("Mtime after update: got %v want 2000", got.Mtime)
	}
}

func TestDeleteTrack(t *testing.T) {
	db := mustOpenDB(t)

	tr := sampleTrack("/music/track2.mp3")
	if err := UpsertTrack(db, tr); err != nil {
		t.Fatalf("UpsertTrack: %v", err)
	}

	if err := DeleteTrack(db, tr.Path); err != nil {
		t.Fatalf("DeleteTrack: %v", err)
	}

	_, err := GetTrack(db, tr.Path)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after delete, got: %v", err)
	}
}

func TestAllTrackPaths(t *testing.T) {
	db := mustOpenDB(t)

	tracks := []TrackRecord{
		{Path: "/a.mp3", Artist: "A", Album: "AA", Title: "T1", Mtime: 100},
		{Path: "/b.mp3", Artist: "B", Album: "BB", Title: "T2", Mtime: 200},
	}
	for _, tr := range tracks {
		if err := UpsertTrack(db, tr); err != nil {
			t.Fatalf("UpsertTrack: %v", err)
		}
	}

	paths, err := AllTrackPaths(db)
	if err != nil {
		t.Fatalf("AllTrackPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths["/a.mp3"] != 100 {
		t.Errorf("mtime for /a.mp3: got %v want 100", paths["/a.mp3"])
	}
	if paths["/b.mp3"] != 200 {
		t.Errorf("mtime for /b.mp3: got %v want 200", paths["/b.mp3"])
	}
}

func seedTracks(t *testing.T, db *sql.DB) {
	t.Helper()
	tracks := []TrackRecord{
		{Path: "/music/rock1.mp3", Artist: "Alice", Album: "Wonderland", Title: "Rabbit Hole", Year: "2020", Genre: "Rock", Duration: 200},
		{Path: "/music/rock2.mp3", Artist: "Alice", Album: "Wonderland", Title: "Mad Hatter", Year: "2020", Genre: "Rock", Duration: 180},
		{Path: "/music/jazz1.mp3", Artist: "Bob", Album: "Blue Note", Title: "Midnight", Year: "2021", Genre: "Jazz", Duration: 300},
		{Path: "/music/jazz2.mp3", Artist: "Bob", Album: "Blue Note", Title: "Sunrise", Year: "2021", Genre: "Jazz", Duration: 250},
		{Path: "/music/pop1.mp3", Artist: "Charlie", Album: "Pop World", Title: "Summer", Year: "2023", Genre: "Pop", Duration: 150},
	}
	for _, tr := range tracks {
		if err := UpsertTrack(db, tr); err != nil {
			t.Fatalf("seed UpsertTrack %s: %v", tr.Path, err)
		}
	}
}

func TestQueryAlbums(t *testing.T) {
	db := mustOpenDB(t)
	seedTracks(t, db)

	// all albums
	albums, err := QueryAlbums(db, nil)
	if err != nil {
		t.Fatalf("QueryAlbums: %v", err)
	}
	if len(albums) != 3 {
		t.Errorf("expected 3 albums, got %d", len(albums))
	}

	// filtered — "blue" should match "Blue Note"
	albums, err = QueryAlbums(db, []string{"blue"})
	if err != nil {
		t.Fatalf("QueryAlbums filtered: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("expected 1 album, got %d", len(albums))
	}
	if albums[0].Album != "Blue Note" {
		t.Errorf("Album: got %q want %q", albums[0].Album, "Blue Note")
	}
	if albums[0].TrackCount != 2 {
		t.Errorf("TrackCount: got %d want 2", albums[0].TrackCount)
	}

	// case-insensitive
	albums, err = QueryAlbums(db, []string{"WONDERLAND"})
	if err != nil {
		t.Fatalf("QueryAlbums case: %v", err)
	}
	if len(albums) != 1 {
		t.Errorf("expected 1 album for WONDERLAND, got %d", len(albums))
	}
}

func TestQueryArtists(t *testing.T) {
	db := mustOpenDB(t)
	seedTracks(t, db)

	artists, err := QueryArtists(db, nil)
	if err != nil {
		t.Fatalf("QueryArtists: %v", err)
	}
	if len(artists) != 3 {
		t.Errorf("expected 3 artists, got %d", len(artists))
	}

	artists, err = QueryArtists(db, []string{"alice"})
	if err != nil {
		t.Fatalf("QueryArtists filtered: %v", err)
	}
	if len(artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(artists))
	}
	if artists[0].Artist != "Alice" {
		t.Errorf("Artist: got %q want %q", artists[0].Artist, "Alice")
	}
	if artists[0].AlbumCount != 1 {
		t.Errorf("AlbumCount: got %d want 1", artists[0].AlbumCount)
	}
	if artists[0].TrackCount != 2 {
		t.Errorf("TrackCount: got %d want 2", artists[0].TrackCount)
	}
}

func TestQueryTracks(t *testing.T) {
	db := mustOpenDB(t)
	seedTracks(t, db)

	// all tracks, sorted
	tracks, err := QueryTracks(db, nil, nil)
	if err != nil {
		t.Fatalf("QueryTracks: %v", err)
	}
	if len(tracks) != 5 {
		t.Errorf("expected 5 tracks, got %d", len(tracks))
	}
	// first should be Alice's track (artist "Alice" < "Bob" < "Charlie")
	if tracks[0].Artist != "Alice" {
		t.Errorf("first track artist: got %q want Alice", tracks[0].Artist)
	}

	// term filter — "midnight" matches title
	tracks, err = QueryTracks(db, []string{"midnight"}, nil)
	if err != nil {
		t.Fatalf("QueryTracks term: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track for 'midnight', got %d", len(tracks))
	}
	if tracks[0].Title != "Midnight" {
		t.Errorf("Title: got %q want Midnight", tracks[0].Title)
	}

	// field filter by year
	tracks, err = QueryTracks(db, nil, map[string][]string{"year": {"2021"}})
	if err != nil {
		t.Fatalf("QueryTracks field filter: %v", err)
	}
	if len(tracks) != 2 {
		t.Errorf("expected 2 tracks for year 2021, got %d", len(tracks))
	}

	// field filter by genre
	tracks, err = QueryTracks(db, nil, map[string][]string{"genre": {"Jazz"}})
	if err != nil {
		t.Fatalf("QueryTracks genre filter: %v", err)
	}
	if len(tracks) != 2 {
		t.Errorf("expected 2 tracks for Jazz, got %d", len(tracks))
	}
}

func TestQueryYears(t *testing.T) {
	db := mustOpenDB(t)
	seedTracks(t, db)

	years, err := QueryYears(db, nil)
	if err != nil {
		t.Fatalf("QueryYears: %v", err)
	}
	// expect 3 distinct years: 2023, 2021, 2020 (DESC)
	if len(years) != 3 {
		t.Errorf("expected 3 years, got %d", len(years))
	}
	if years[0].Year != "2023" {
		t.Errorf("first year: got %q want 2023", years[0].Year)
	}

	// filtered
	years, err = QueryYears(db, []string{"2021"})
	if err != nil {
		t.Fatalf("QueryYears filtered: %v", err)
	}
	if len(years) != 1 || years[0].Year != "2021" {
		t.Errorf("filtered years: %+v", years)
	}
}

func TestQueryGenres(t *testing.T) {
	db := mustOpenDB(t)
	seedTracks(t, db)

	genres, err := QueryGenres(db, nil)
	if err != nil {
		t.Fatalf("QueryGenres: %v", err)
	}
	if len(genres) != 3 {
		t.Errorf("expected 3 genres, got %d", len(genres))
	}

	genres, err = QueryGenres(db, []string{"jazz"})
	if err != nil {
		t.Fatalf("QueryGenres filtered: %v", err)
	}
	if len(genres) != 1 || genres[0].Genre != "Jazz" {
		t.Errorf("filtered genres: %+v", genres)
	}
	if genres[0].TrackCount != 2 {
		t.Errorf("TrackCount: got %d want 2", genres[0].TrackCount)
	}
}

func TestQueryAlbumsWithYear(t *testing.T) {
	db := mustOpenDB(t)
	seedTracks(t, db)

	albums, err := QueryAlbumsWithYear(db, "2020")
	if err != nil {
		t.Fatalf("QueryAlbumsWithYear: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("expected 1 album for 2020, got %d", len(albums))
	}
	if albums[0].Album != "Wonderland" {
		t.Errorf("Album: got %q want Wonderland", albums[0].Album)
	}
}
