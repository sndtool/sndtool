package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS tracks (
    path     TEXT PRIMARY KEY,
    artist   TEXT NOT NULL DEFAULT '',
    album    TEXT NOT NULL DEFAULT '',
    title    TEXT NOT NULL DEFAULT '',
    year     TEXT NOT NULL DEFAULT '',
    genre    TEXT NOT NULL DEFAULT '',
    duration REAL NOT NULL DEFAULT 0,
    mtime    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS playlists (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT NOT NULL UNIQUE,
    created  INTEGER NOT NULL,
    updated  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_path  TEXT NOT NULL REFERENCES tracks(path) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, track_path)
);

CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist);
CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album);
CREATE INDEX IF NOT EXISTS idx_tracks_year ON tracks(year);
CREATE INDEX IF NOT EXISTS idx_tracks_genre ON tracks(genre);
`

// OpenDB opens (or creates) a SQLite database at dsn, enables foreign keys,
// and applies the application schema. Returns the open *sql.DB.
func OpenDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}

// --- Types ---

// TrackRecord holds metadata for a single audio track.
type TrackRecord struct {
	Path, Artist, Album, Title, Year, Genre string
	Duration                                float64
	Mtime                                   int64
}

// AlbumResult holds aggregated data for an album.
type AlbumResult struct {
	Album      string
	Artist     string
	TrackCount int
	Duration   float64
}

// ArtistResult holds aggregated data for an artist.
type ArtistResult struct {
	Artist     string
	AlbumCount int
	TrackCount int
}

// YearResult holds aggregated data for a release year.
type YearResult struct {
	Year       string
	TrackCount int
}

// GenreResult holds aggregated data for a genre.
type GenreResult struct {
	Genre      string
	TrackCount int
}

// PlaylistResult holds summary data for a playlist.
type PlaylistResult struct {
	ID         int64
	Name       string
	TrackCount int
	Created    int64
	Updated    int64
}

// --- Track CRUD ---

// UpsertTrack inserts a track or updates it if the path already exists.
func UpsertTrack(db *sql.DB, t TrackRecord) error {
	_, err := db.Exec(`
		INSERT INTO tracks (path, artist, album, title, year, genre, duration, mtime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			artist   = excluded.artist,
			album    = excluded.album,
			title    = excluded.title,
			year     = excluded.year,
			genre    = excluded.genre,
			duration = excluded.duration,
			mtime    = excluded.mtime`,
		t.Path, t.Artist, t.Album, t.Title, t.Year, t.Genre, t.Duration, t.Mtime,
	)
	if err != nil {
		return fmt.Errorf("upsert track %q: %w", t.Path, err)
	}
	return nil
}

// GetTrack returns the track with the given path, or sql.ErrNoRows if not found.
func GetTrack(db *sql.DB, path string) (TrackRecord, error) {
	var t TrackRecord
	err := db.QueryRow(`
		SELECT path, artist, album, title, year, genre, duration, mtime
		FROM tracks WHERE path = ?`, path,
	).Scan(&t.Path, &t.Artist, &t.Album, &t.Title, &t.Year, &t.Genre, &t.Duration, &t.Mtime)
	if err != nil {
		return TrackRecord{}, err
	}
	return t, nil
}

// DeleteTrack removes the track with the given path.
func DeleteTrack(db *sql.DB, path string) error {
	_, err := db.Exec("DELETE FROM tracks WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("delete track %q: %w", path, err)
	}
	return nil
}

// AllTrackPaths returns a map of path → mtime for all tracks in the database.
func AllTrackPaths(db *sql.DB) (map[string]int64, error) {
	rows, err := db.Query("SELECT path, mtime FROM tracks")
	if err != nil {
		return nil, fmt.Errorf("all track paths: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var path string
		var mtime int64
		if err := rows.Scan(&path, &mtime); err != nil {
			return nil, fmt.Errorf("scan track path: %w", err)
		}
		result[path] = mtime
	}
	return result, rows.Err()
}

// likeFilters builds AND clauses where each term must match at least one of the
// given columns via case-insensitive LIKE. Returns the WHERE clause fragment
// (without "WHERE") and the args slice.
func likeFilters(terms []string, columns ...string) (string, []interface{}) {
	if len(terms) == 0 || len(columns) == 0 {
		return "", nil
	}

	var clauses []string
	var args []interface{}

	for _, term := range terms {
		pattern := "%" + strings.ToLower(term) + "%"
		var orParts []string
		for _, col := range columns {
			orParts = append(orParts, fmt.Sprintf("LOWER(%s) LIKE ?", col))
			args = append(args, pattern)
		}
		clauses = append(clauses, "("+strings.Join(orParts, " OR ")+")")
	}

	return strings.Join(clauses, " AND "), args
}

// QueryAlbums returns albums optionally filtered by terms matching album name.
func QueryAlbums(db *sql.DB, terms []string) ([]AlbumResult, error) {
	q := `SELECT album, artist, COUNT(*) AS track_count, SUM(duration) AS total_duration
		FROM tracks`
	clause, args := likeFilters(terms, "album")
	if clause != "" {
		q += " WHERE " + clause
	}
	q += " GROUP BY album ORDER BY album"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer rows.Close()

	var results []AlbumResult
	for rows.Next() {
		var r AlbumResult
		if err := rows.Scan(&r.Album, &r.Artist, &r.TrackCount, &r.Duration); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryArtists returns artists optionally filtered by terms matching artist name.
func QueryArtists(db *sql.DB, terms []string) ([]ArtistResult, error) {
	q := `SELECT artist, COUNT(DISTINCT album) AS album_count, COUNT(*) AS track_count
		FROM tracks`
	clause, args := likeFilters(terms, "artist")
	if clause != "" {
		q += " WHERE " + clause
	}
	q += " GROUP BY artist ORDER BY artist"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query artists: %w", err)
	}
	defer rows.Close()

	var results []ArtistResult
	for rows.Next() {
		var r ArtistResult
		if err := rows.Scan(&r.Artist, &r.AlbumCount, &r.TrackCount); err != nil {
			return nil, fmt.Errorf("scan artist: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryTracks returns tracks filtered by terms (matching any of artist/album/title)
// and fieldFilters (restricting specific fields). Results are ordered by artist, album, title.
func QueryTracks(db *sql.DB, terms []string, fieldFilters map[string][]string) ([]TrackRecord, error) {
	q := `SELECT path, artist, album, title, year, genre, duration, mtime FROM tracks`

	var conditions []string
	var args []interface{}

	// term filters: each term must match artist OR album OR title
	termClause, termArgs := likeFilters(terms, "artist", "album", "title")
	if termClause != "" {
		conditions = append(conditions, termClause)
		args = append(args, termArgs...)
	}

	// field filters: exact field restrictions
	allowedFields := map[string]bool{"artist": true, "album": true, "title": true, "year": true, "genre": true}
	for field, values := range fieldFilters {
		if !allowedFields[field] {
			continue
		}
		var orParts []string
		for _, v := range values {
			orParts = append(orParts, fmt.Sprintf("LOWER(%s) = LOWER(?)", field))
			args = append(args, v)
		}
		conditions = append(conditions, "("+strings.Join(orParts, " OR ")+")")
	}

	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += " ORDER BY artist, album, title"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query tracks: %w", err)
	}
	defer rows.Close()

	var results []TrackRecord
	for rows.Next() {
		var t TrackRecord
		if err := rows.Scan(&t.Path, &t.Artist, &t.Album, &t.Title, &t.Year, &t.Genre, &t.Duration, &t.Mtime); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// QueryYears returns distinct years ordered descending, optionally filtered by terms.
func QueryYears(db *sql.DB, terms []string) ([]YearResult, error) {
	q := `SELECT year, COUNT(*) AS track_count FROM tracks`
	clause, args := likeFilters(terms, "year")
	if clause != "" {
		q += " WHERE " + clause
	}
	q += " GROUP BY year ORDER BY year DESC"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query years: %w", err)
	}
	defer rows.Close()

	var results []YearResult
	for rows.Next() {
		var r YearResult
		if err := rows.Scan(&r.Year, &r.TrackCount); err != nil {
			return nil, fmt.Errorf("scan year: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryGenres returns distinct genres ordered alphabetically, optionally filtered by terms.
func QueryGenres(db *sql.DB, terms []string) ([]GenreResult, error) {
	q := `SELECT genre, COUNT(*) AS track_count FROM tracks`
	clause, args := likeFilters(terms, "genre")
	if clause != "" {
		q += " WHERE " + clause
	}
	q += " GROUP BY genre ORDER BY genre"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query genres: %w", err)
	}
	defer rows.Close()

	var results []GenreResult
	for rows.Next() {
		var r GenreResult
		if err := rows.Scan(&r.Genre, &r.TrackCount); err != nil {
			return nil, fmt.Errorf("scan genre: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryAlbumsWithYear returns albums released in the given year.
func QueryAlbumsWithYear(db *sql.DB, year string) ([]AlbumResult, error) {
	rows, err := db.Query(`
		SELECT album, artist, COUNT(*) AS track_count, SUM(duration) AS total_duration
		FROM tracks
		WHERE year = ?
		GROUP BY album
		ORDER BY album`, year)
	if err != nil {
		return nil, fmt.Errorf("query albums with year: %w", err)
	}
	defer rows.Close()

	var results []AlbumResult
	for rows.Next() {
		var r AlbumResult
		if err := rows.Scan(&r.Album, &r.Artist, &r.TrackCount, &r.Duration); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- Playlist CRUD ---

// CreatePlaylist creates a new playlist with the given name and returns its ID.
func CreatePlaylist(db *sql.DB, name string) (int64, error) {
	now := time.Now().Unix()
	res, err := db.Exec(
		"INSERT INTO playlists (name, created, updated) VALUES (?, ?, ?)",
		name, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create playlist %q: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// DeletePlaylist deletes the playlist with the given ID (cascade deletes playlist_tracks).
func DeletePlaylist(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM playlists WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete playlist %d: %w", id, err)
	}
	return nil
}

// RenamePlaylist renames the playlist with the given ID.
func RenamePlaylist(db *sql.DB, id int64, name string) error {
	now := time.Now().Unix()
	_, err := db.Exec(
		"UPDATE playlists SET name = ?, updated = ? WHERE id = ?",
		name, now, id,
	)
	if err != nil {
		return fmt.Errorf("rename playlist %d: %w", id, err)
	}
	return nil
}

// ListPlaylists returns playlists optionally filtered by terms matching the name.
func ListPlaylists(db *sql.DB, terms []string) ([]PlaylistResult, error) {
	q := `SELECT p.id, p.name, COUNT(pt.track_path) AS track_count, p.created, p.updated
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.id`

	clause, args := likeFilters(terms, "p.name")
	if clause != "" {
		q += " WHERE " + clause
	}
	q += " GROUP BY p.id ORDER BY p.name"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list playlists: %w", err)
	}
	defer rows.Close()

	var results []PlaylistResult
	for rows.Next() {
		var r PlaylistResult
		if err := rows.Scan(&r.ID, &r.Name, &r.TrackCount, &r.Created, &r.Updated); err != nil {
			return nil, fmt.Errorf("scan playlist: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// AddToPlaylist appends the given paths to a playlist, auto-incrementing position.
func AddToPlaylist(db *sql.DB, playlistID int64, paths []string) error {
	// find current max position
	var maxPos int
	err := db.QueryRow(
		"SELECT COALESCE(MAX(position), -1) FROM playlist_tracks WHERE playlist_id = ?",
		playlistID,
	).Scan(&maxPos)
	if err != nil {
		return fmt.Errorf("get max position: %w", err)
	}

	now := time.Now().Unix()
	for i, path := range paths {
		_, err := db.Exec(
			"INSERT OR IGNORE INTO playlist_tracks (playlist_id, track_path, position) VALUES (?, ?, ?)",
			playlistID, path, maxPos+1+i,
		)
		if err != nil {
			return fmt.Errorf("add %q to playlist %d: %w", path, playlistID, err)
		}
	}

	// update playlist updated timestamp
	_, err = db.Exec("UPDATE playlists SET updated = ? WHERE id = ?", now, playlistID)
	return err
}

// RemoveFromPlaylist removes the given paths from a playlist.
func RemoveFromPlaylist(db *sql.DB, playlistID int64, paths []string) error {
	now := time.Now().Unix()
	for _, path := range paths {
		_, err := db.Exec(
			"DELETE FROM playlist_tracks WHERE playlist_id = ? AND track_path = ?",
			playlistID, path,
		)
		if err != nil {
			return fmt.Errorf("remove %q from playlist %d: %w", path, playlistID, err)
		}
	}
	_, err := db.Exec("UPDATE playlists SET updated = ? WHERE id = ?", now, playlistID)
	return err
}

// GetPlaylistTracks returns tracks in a playlist ordered by position.
func GetPlaylistTracks(db *sql.DB, playlistID int64) ([]TrackRecord, error) {
	rows, err := db.Query(`
		SELECT t.path, t.artist, t.album, t.title, t.year, t.genre, t.duration, t.mtime
		FROM playlist_tracks pt
		JOIN tracks t ON t.path = pt.track_path
		WHERE pt.playlist_id = ?
		ORDER BY pt.position`, playlistID)
	if err != nil {
		return nil, fmt.Errorf("get playlist tracks: %w", err)
	}
	defer rows.Close()

	var results []TrackRecord
	for rows.Next() {
		var t TrackRecord
		if err := rows.Scan(&t.Path, &t.Artist, &t.Album, &t.Title, &t.Year, &t.Genre, &t.Duration, &t.Mtime); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		results = append(results, t)
	}
	return results, rows.Err()
}
