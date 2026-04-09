# sndtool Library Mode Design

## Overview

Add a library browsing mode to sndtool backed by a SQLite database (`sndtool.db`).
The library provides artist/album/genre/year/playlist browsing via a command-driven
query language, while the existing file-browser mode is preserved for direct file
management. A background scanner keeps the database in sync with files on disk.

## Two Modes

sndtool operates in one of two modes, toggled with `v`:

- **Library mode** — browse by metadata using the `:` query language. Requires
  `sndtool.db`.
- **File browser mode** — existing directory/file browsing. No database required.
  Uses `/` for filtering.

Pressing `v` switches between modes:
- From library → file browser:
  - On a track: opens the directory containing that track.
  - On an album: opens the directory containing the album's tracks.
  - On an artist/genre/year: opens file browser at the root music directory.
- From file browser → library: returns to the last query/view in library mode.

Playback (mpv) is shared across both modes. The playback status bar at the bottom
persists regardless of mode.

## Startup Flow

1. Check if `sndtool.db` exists in the target directory.
2. If yes: open DB, start in library mode (default view: `:album` — all albums),
   launch background scanner.
3. If no: prompt `"Create library database? (y/n)"`.
   - `y`: create `sndtool.db`, run initial scan (with progress), start in library mode.
   - `n`: start in file browser mode. `v` key is disabled (no DB).

The database file is named `sndtool.db` (not hidden) and lives in the root music
directory passed to sndtool.

## Database Schema

SQLite database with three tables:

```sql
CREATE TABLE tracks (
    path     TEXT PRIMARY KEY,  -- full filesystem path
    artist   TEXT NOT NULL DEFAULT '',
    album    TEXT NOT NULL DEFAULT '',
    title    TEXT NOT NULL DEFAULT '',
    year     TEXT NOT NULL DEFAULT '',
    genre    TEXT NOT NULL DEFAULT '',
    duration REAL NOT NULL DEFAULT 0,  -- seconds
    mtime    INTEGER NOT NULL DEFAULT 0  -- file modification time (unix)
);

CREATE TABLE playlists (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT NOT NULL UNIQUE,
    created  INTEGER NOT NULL,  -- unix timestamp
    updated  INTEGER NOT NULL   -- unix timestamp
);

CREATE TABLE playlist_tracks (
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_path  TEXT NOT NULL REFERENCES tracks(path) ON DELETE CASCADE,
    position    INTEGER NOT NULL,  -- ordering within playlist
    PRIMARY KEY (playlist_id, track_path)
);

-- Indexes for common queries
CREATE INDEX idx_tracks_artist ON tracks(artist);
CREATE INDEX idx_tracks_album ON tracks(album);
CREATE INDEX idx_tracks_year ON tracks(year);
CREATE INDEX idx_tracks_genre ON tracks(genre);
```

## Background Scanner

The scanner runs as a goroutine after startup and keeps the database current.

### Behavior

1. Walk the directory tree from the root music directory.
2. For each `.mp3` file:
   - If not in DB: read ID3 tags, insert record.
   - If in DB but mtime differs: re-read tags, update record.
   - If in DB and mtime matches: skip.
3. For each DB record: if file no longer exists on disk, delete the record.
4. Send a message to the TUI when scan completes so the current view can refresh.

### Properties

- Runs in background — TUI is usable immediately with existing DB data.
- Incremental — only re-reads tags for changed files.
- Non-blocking — uses a channel or tea.Cmd to notify the TUI of updates.
- On initial scan (new DB), show a progress indicator in the status bar.

## Query Language

The `:` key opens a command prompt in library mode. The query language uses a
simple grammar: `[view] [terms...] [field:terms...]`.

### Parsing Rules

1. The first recognized keyword becomes the view command.
2. Words between the view keyword and the next field keyword are general filter
   terms for the view (all must match).
3. A field keyword captures all subsequent words until the next field keyword.
4. If no view keyword is present, display mixed results (artists, albums, tracks).
5. All matching is case-insensitive substring matching.

### View Keywords

These determine how results are grouped and displayed:

| Keyword    | Display                        |
|------------|--------------------------------|
| `artist`   | List of artists                |
| `album`    | List of albums                 |
| `year`     | List of years                  |
| `genre`    | List of genres                 |
| `playlist` | List of playlists              |
| `track`    | Flat list of individual tracks |
| (none)     | Mixed results: artists, albums, then tracks in sections |

### Filter Terms

Multiple bare words after a view keyword filter within that view (all must match):
- `:album sermon` — albums matching "sermon"
- `:album sunday sermon` — albums matching both "sunday" and "sermon"
- `:artist smith` — artists matching "smith"

Field keywords narrow by a specific field when combined. Multiple words after a
field keyword all apply to that field:
- `:album sermon year 2025` — albums matching "sermon" from 2025
- `:track artist smith david year 2025` — tracks by artist matching "smith" and "david" from 2025
- `:album sunday sermon artist johnson` — albums matching "sunday" and "sermon" by artist "johnson"

### Bare Text Search

A query with no view keyword (e.g., `:sermon on hope`) searches all fields and
displays mixed results in sections:

1. **Artists** matching the query
2. **Albums** matching the query
3. **Tracks** matching the query

Each section has a header. The user scrolls to the level they care about. Enter
on an artist drills to their albums, enter on an album drills to its tracks,
enter on a track plays it.

### Highlighting

- **Command line:** view keywords in one color, filter field keywords in another
  color, search terms in a third color.
- **Results:** matching search terms highlighted in result rows (similar to
  existing `matchStyle` yellow highlight).

### Tab Completion

As the user types after `:`, a dropdown shows matching completions:
- Keywords (`album`, `artist`, `genre`, `year`, `playlist`, `track`)
- Actual values from the database (artist names, album names, etc.)

Tab fills the selected completion. Arrow keys navigate the dropdown. Keywords
are visually distinguished from data values.

### Query Persistence

- Enter commits the query — results display and the query remains visible at
  the top of the screen.
- Pressing `:` again re-opens the query for editing (cursor at end).
- `Esc` clears the query and returns to the default library view.
- `h`/backspace when drilled into a group goes back one level (e.g., tracks →
  album list) without clearing the query.

## Library Mode Display

### Group Views (artist, album, year, genre)

Display as a columnar list. No individual tracks shown — only aggregated groups.

Example — `:album sermon`:

```
:album sermon
  Album                    Artist           Tracks  Duration
─────────────────────────────────────────────────────────────
  Sunday Sermons 2024      Johnson, Mark    24      4:12:30
> Sunday Sermons 2025      Johnson, Mark    18      3:45:00
  Sermon Series Vol 1      Smith, David      8      1:30:15
─────────────────────────────────────────────────────────────
  [3 albums] enter to expand, : to edit query
```

### Drill-Down

Enter on a group drills down one level:
- Artist → their albums
- Album → tracks in that album
- Year → albums from that year
- Genre → albums in that genre

The breadcrumb trail shows in the query line:
`:album sermon › Sunday Sermons 2025`

Backspace goes back up one level.

### Track View

Displayed when:
1. Using the `track` keyword explicitly
2. Drilling into a specific album
3. Opening a playlist

Tracks are sorted by Artist, Album, Title (in that order). Columns are displayed
in the same order: Artist, Album, Title, Year, Name. This puts the most useful
grouping/browsing fields first and the filename last.

### Mixed Search Results

For bare text queries, results are displayed in sections with headers:

```
:sermon on hope
  Artists
  ─────────────────────────────────
  Johnson, Mark                          3 albums

  Albums
  ─────────────────────────────────
  Sunday Sermons 2024      Johnson, Mark    24 tracks
> Sunday Sermons 2025      Johnson, Mark    18 tracks

  Tracks
  ─────────────────────────────────
  Smith, David       Sermon Series    Sermon on Hope     04-sermon-hope.mp3
  Johnson, Mark      Sunday Sermons   Hope Renewed       12-hope-renewed.mp3
```

Matching terms ("sermon", "hope") are highlighted in the results.

## Playlists

### Storage

Playlists are stored in `sndtool.db` (tables `playlists` and `playlist_tracks`).
No M3U support initially — can be added later.

### Browsing

- `:playlist` — list all playlists with track count
- `:playlist favorites` — filter playlists matching "favorites"
- Enter on a playlist — show its tracks (ordered by position)

### Adding to Playlists

Press `a` on a track, album, or marked items to add to a playlist:

1. A picker overlay appears showing:
   - `+ New playlist...` at the top
   - Existing playlists with track counts
2. Navigate with `j`/`k`, select with Enter, cancel with Esc.
3. Selecting "New playlist" prompts for a name, then adds the items.
4. Adding an album adds all its tracks in album order.
5. Marked items (`space`) are added as a batch.

### Managing Playlists

When viewing a playlist's tracks:
- `d` — remove selected/marked tracks (with confirmation)
- `r` — rename the playlist
- `P` — play all tracks in playlist order

Deleting a playlist itself: `d` when the cursor is on a playlist in the
`:playlist` list view.

## Key Bindings

### New Keys

| Key | Context        | Action                              |
|-----|----------------|-------------------------------------|
| `v` | Both modes     | Toggle Library / File Browser       |
| `:` | Library mode   | Open/edit query prompt              |
| `a` | Both modes     | Add track/album/marked to playlist  |

### Existing Keys in Library Mode

All existing navigation keys work in library mode with the same semantics:

| Key            | Action in library mode                     |
|----------------|--------------------------------------------|
| `j`/`k`, arrows | Navigate list                            |
| `enter`        | Drill into group / play track              |
| `h`/backspace  | Go back one level                          |
| `space`        | Mark items                                 |
| `P`            | Play track                                 |
| `e`            | Edit tags (on tracks)                      |
| `S`            | Pause/resume playback                      |
| `Shift+arrows` | Seek / prev/next track                     |
| `+`/`-`        | Volume                                     |
| `Esc`          | Clear query / exit library mode            |
| `q`            | Quit                                       |

### Keys NOT Available in Library Mode

| Key | Reason                                      |
|-----|---------------------------------------------|
| `/` | Not needed — `:` handles all filtering      |
| `f` | Find is a file-browser concept              |
| `c`/`x`/`p` | Clipboard is file-browser only       |
| `m` | Merge is file-browser only                  |
| `~` | Home directory is file-browser concept      |

## File Browser Mode

Unchanged from current behavior. All existing keybindings preserved. The `a` key
is added for playlist support (requires DB to exist).

## Architecture Notes

### Code Organization

The current `tui.go` is ~2200 lines. Adding library mode will require significant
new code. Recommended split:

- `cmd/tui.go` — shared model, mode dispatch, playback, common rendering
- `cmd/tui_browse.go` — file browser mode (extract existing code)
- `cmd/tui_library.go` — library mode, query parsing, completion
- `cmd/tui_playlist.go` — playlist picker, playlist view
- `cmd/db.go` — SQLite operations, schema, scanner
- `cmd/query.go` — query language parser

### Dependencies

New dependency: `modernc.org/sqlite` (pure Go SQLite, no CGo) or
`github.com/mattn/go-sqlite3` (CGo). The pure Go option avoids cross-compilation
issues.

### Data Flow

1. Scanner populates/updates `sndtool.db` in background.
2. Query parser converts `:` input into SQL queries against the DB.
3. Results are loaded into `[]tagEntry` (same struct used in file browser).
4. Rendering reuses existing columnar display code where possible.
5. Drill-down maintains a stack of query states for back-navigation.
