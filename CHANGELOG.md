# Changelog

## Unreleased

## v0.2.0 (2026-04-15)

- Fix: pgdn/pgup/ctrl-f/ctrl-b now work in queue view for page scrolling
- Fix: space key now works in library query editor and playlist name input
- TUI: library mode — `:` query prompt with keyword tab-completion, syntax
  highlighting; query execution (album/artist/track/year/genre/playlist/mixed);
  drill-down navigation (enter to open, h/backspace to go back); breadcrumb
  trail; play/append-to-queue from results; space to mark, search term
  highlighting
- TUI: three view modes (Library/Queue/Files) — cycle with `tab`/`shift+tab`
- TUI: play queue integration — `P` builds queue from visible entries, `A`
  appends to queue, Shift+Up/Down navigate queue, auto-advance uses queue
- TUI: flash only the speaker emoji instead of the entire line during playback
- TUI: queue view — columnar track list with 🔊 playing indicator, cursor
  navigation (j/k), space-to-mark, d-to-remove, P-to-jump-to-play
- TUI: startup flow — detects `sndtool.db` in target directory; if absent,
  prompts "Create library database? (y/n)"; opens/creates DB, runs background
  scanner on init
- Library mode: SQLite-backed browsing by artist, album, year, genre, playlist
- Library mode: command-driven query language with tab completion
- Library mode: mixed search across artists, albums, and tracks
- Play queue: independent playback queue persists across view changes
- Play queue: P replaces queue, A appends, Shift+Up/Down navigates queue
- Playlists: create, rename, delete; add/remove tracks
- Three view modes: Files, Library, Queue — cycle with tab
- Fix tab bar being pushed off screen in library view (chrome height
  calculation)
- Add tab bar to all shared views (detail, edit, confirm, rename, playlist
  picker, queue)
- Background scanner keeps library database in sync with files on disk

## v0.1.2 (2026-04-08)

- TUI: `Q` quality check — recursively finds MP3 files missing artist, album, or
  title tags; displays results in columnar browse view with full edit/play
  support
- TUI: find (`f`) now uses the same columnar browse view; all browse keybindings
  (edit, play, mark, etc.) work in find results; search input at bottom like
  filter
- TUI: filter (`/`) works inside find results for two-level search
- TUI: increase max find results from 200 to 2000
- TUI: skip non-playable files and directories when using Shift+↑/↓ and
  auto-advance; wrap around at start/end of list
- TUI: fix Shift+↑/↓ not skipping back to the currently playing file
- TUI: show audio file length (h:m:s) in detail and edit views
- TUI: filter in find mode also matches against full file path
- Dev: add `st_format` to envsetup.sh for formatting Go code

## v0.1.0 (2026-04-08)

- TUI: audio playback via mpv — `P` plays current file with progress bar,
  artist/title display, and auto-advance to next track; `S` pauses/resumes;
  `Shift+←/→` seeks ±10s; `Shift+↑/↓` skips to prev/next song; `+`/`-`/`=`
  adjusts volume (persisted across tracks); shows mpv install instructions if
  not found
- TUI: dynamic column widths scale to terminal width
- Docs: add screenshot to README; document mpv dependency
- TUI: fix header/location line being pushed off screen by long directory
  listings
- TUI: `f` opens fuzzy finder that recursively searches files/dirs by name or
  tags
- TUI: `b` navigates back to previous directory before a jump
- TUI: `~` returns to the start directory
- TUI: edit mode (`e`) now includes a Name field to rename files and directories
- TUI: filter (`/`) now filters the list to matching entries instead of
  highlighting and jumping between matches; removed `n`/`N` navigation
- TUI: `esc` now clears active filter first; second `esc` quits
- TUI: renamed `/` from "search" to "filter" in UI labels and docs

## v0.0.6 (2026-04-08)

- TUI: add Album column to browse view
- Docs: add Contributing section to README with links to Issues, PRs, and
  Discussions

## v0.0.5 (2026-03-13)

- Fix: handle corrupt ID3 tags ("frame went over tag area") by stripping and
  rewriting the tag on save instead of failing

## v0.0.4 (2026-03-13)

- TUI: PgDn/PgUp and Ctrl-f/Ctrl-b for page scrolling in browse view

## v0.0.3 (2026-03-13)

### Breaking Changes

- Remove `tags` subcommand — TUI is now the default when running `sndtool` with
  no command (or `sndtool [directory]`)

### Enhancements

- TUI: Add viewport scrolling so file lists longer than the terminal are
  navigable
- TUI: Show directories in the file browser and support drilling into them
  (Enter/l) and navigating to parent (Backspace/h)
- TUI: Display current directory path in header and scroll position indicator
- TUI: Use alt-screen for proper full-screen display
- TUI: Enter on a file shows tag detail view (Artist, Album, Title, Year, path)
- TUI: `e` on a file opens inline tag editor
- TUI: `e` on a directory edits common tags (Artist, Album, Year) across all
  MP3s in that directory
- TUI: `d` deletes files/directories with y/n confirmation
- TUI: `space` marks/unmarks files for batch operations
- TUI: `c` copies current or marked items to clipboard, `p` pastes into current
  directory
- TUI: Status messages for copy/paste/save/delete feedback
- TUI: `x` cuts (marks for move) current or marked items, `p` pastes via move
- TUI: `b` merges all MP3s in a directory into a single file (renamed to `m`)
- TUI: `r` renames the current file or directory inline
- TUI: Left/Right arrow keys for horizontal scrolling in browse view

## v0.0.2 (2026-03-13)

### Bug Fixes

- Fix goreleaser build config to specify `./cmd` as the main package entry point
- Update Go module path from `github.com/cbrake/sndtool` to
  `github.com/sndtool/sndtool`
- Update GitHub API and release URLs to use the new `sndtool/sndtool`
  organization/repo
- Update goreleaser footer to reference `sndtool update` instead of
  `soundrig update`

### Other

- Add project icon (`sndtool-icon.png`)

## v0.0.1 (2026-03-13)

Initial release — renamed from `soundrig` to `sndtool`.
