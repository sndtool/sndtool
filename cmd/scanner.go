package main

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"

	id3 "github.com/bogem/id3v2/v2"
)

// ScanStats holds counters from a ScanDir run.
type ScanStats struct {
	Added   int
	Updated int
	Deleted int
	Skipped int
}

// ScanDir scans root for MP3 files and syncs them into db.
// Files not in DB are added; files with a changed mtime are updated;
// DB records for files no longer on disk are deleted.
func ScanDir(db *sql.DB, root string) (ScanStats, error) {
	existing, err := AllTrackPaths(db)
	if err != nil {
		return ScanStats{}, err
	}

	// seen tracks paths encountered on disk during this walk.
	seen := make(map[string]struct{}, len(existing))

	var stats ScanStats

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip entries we cannot access.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".mp3") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			// Skip files we cannot stat.
			return nil
		}
		mtime := info.ModTime().Unix()

		seen[path] = struct{}{}

		dbMtime, inDB := existing[path]
		switch {
		case !inDB:
			t := readTrackTags(path, mtime)
			if uErr := UpsertTrack(db, t); uErr != nil {
				// Log and continue rather than aborting the whole scan.
				return nil
			}
			stats.Added++
		case dbMtime != mtime:
			t := readTrackTags(path, mtime)
			if uErr := UpsertTrack(db, t); uErr != nil {
				return nil
			}
			stats.Updated++
		default:
			stats.Skipped++
		}

		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}

	// Delete DB records for files that were not seen on disk.
	for path := range existing {
		if _, ok := seen[path]; !ok {
			if dErr := DeleteTrack(db, path); dErr == nil {
				stats.Deleted++
			}
		}
	}

	return stats, nil
}

// readTrackTags opens path and reads its ID3 tags into a TrackRecord.
// On any error, returns a TrackRecord with only Path and Mtime populated.
func readTrackTags(path string, mtime int64) TrackRecord {
	t := TrackRecord{
		Path:  path,
		Mtime: mtime,
	}

	tag, err := id3.Open(path, id3.Options{Parse: true})
	if err != nil {
		return t
	}
	defer tag.Close()

	t.Artist = tag.Artist()
	t.Album = tag.Album()
	t.Title = tag.Title()
	t.Year = tag.Year()
	t.Genre = tag.Genre()

	return t
}
