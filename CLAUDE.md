# sndtool

A keyboard-driven TUI + CLI for audio file management (browse, tag, merge MP3s).

## Build & Run

```
go build -o sndtool .        # build
./sndtool [directory]         # launch TUI
./sndtool merge <directory>   # merge MP3s via CLI
```

## Project Structure

- `cmd/` — CLI entry point (`main.go`), TUI logic (`tui.go`), self-update (`update.go`)
- `merge.go` — MP3 merging and auto-tagging logic
- `scripts/` — helper scripts (changelog, etc.)

## Stack

- Go, Bubble Tea (TUI), Lip Gloss (styling), id3v2 (tags), mp3lib (frame-level MP3)

## TUI Keybindings

| Key | Action |
|-----|--------|
| `j`/`k`, `up`/`down` | Navigate |
| `enter` | Open directory / view tags |
| `l`/`right` | Enter directory |
| `h`/`backspace` | Parent directory |
| `e` | Edit tags |
| `d` | Delete (with confirmation) |
| `space` | Mark/unmark |
| `c` | Copy |
| `x` | Cut (mark for move) |
| `p` | Paste (copy or move) |
| `m` | Merge MP3s in directory |
| `r` | Rename |
| `q`/`esc` | Quit |

## Docs Policy

After any code changes, update these files to stay in sync:

- `README.md` — user-facing features, keybindings, usage
- `CLAUDE.md` — keybindings table, conventions, project structure
- `CHANGELOG.md` — add entry under the current unreleased version

## Conventions

- TUI modes: `modeBrowse`, `modeDetail`, `modeEdit`, `modeEditDir`, `modeConfirm`, `modeRename`
- File operations use `getMarkedOrCurrent()` to work on marked items or cursor item
- Clipboard is an in-memory `[]string` of file paths; `clipboardCut` flag distinguishes copy vs cut
- Cut (`x`) marks files for move without deleting; paste (`p`) uses `os.Rename` (with cross-device fallback)
- Delete always requires confirmation via `modeConfirm`
