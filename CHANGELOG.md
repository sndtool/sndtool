# Changelog

## Unreleased

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
