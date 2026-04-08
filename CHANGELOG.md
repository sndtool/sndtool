# Changelog

## Unreleased

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
