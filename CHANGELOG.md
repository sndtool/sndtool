# Changelog

## v0.0.3 (unreleased)

### Enhancements

- TUI: Add viewport scrolling so file lists longer than the terminal are navigable
- TUI: Show directories in the file browser and support drilling into them (Enter/l) and navigating to parent (Backspace/h)
- TUI: Display current directory path in header and scroll position indicator
- TUI: Use alt-screen for proper full-screen display

## v0.0.2 (2026-03-13)

### Bug Fixes

- Fix goreleaser build config to specify `./cmd` as the main package entry point
- Update Go module path from `github.com/cbrake/sndtool` to `github.com/sndtool/sndtool`
- Update GitHub API and release URLs to use the new `sndtool/sndtool` organization/repo
- Update goreleaser footer to reference `sndtool update` instead of `soundrig update`

### Other

- Add project icon (`sndtool-icon.png`)

## v0.0.1 (2026-03-13)

Initial release — renamed from `soundrig` to `sndtool`.
