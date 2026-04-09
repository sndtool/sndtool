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
| `e` | Edit name and tags |
| `d` | Delete (with confirmation) |
| `space` | Mark/unmark |
| `c` | Copy |
| `x` | Cut (mark for move) |
| `p` | Paste (copy or move) |
| `m` | Merge MP3s in directory |
| `r` | Rename |
| `/` | Filter (filters list live) |
| `f` | Fuzzy find (recursive search from start dir) |
| `b` | Back to previous directory (before jump) |
| `~` | Home (return to start directory) |
| `Q` | Quality check — find files with missing tags |
| `P` | Play file with mpv |
| `S` | Pause/resume playback |
| `Shift+←/→` | Seek backward/forward 10s |
| `Shift+↑/↓` | Previous/next song |
| `A` | Append tracks to play queue |
| `v` | Cycle view mode (Files/Library/Queue) |
| `+`/`-` | Volume up/down |
| `pgdn`/`ctrl-f` | Page down |
| `pgup`/`ctrl-b` | Page up |
| `esc` | Clear filter (if active), otherwise quit |
| `q` | Quit |

## Docs Policy

After any code changes, update these files to stay in sync:

- `README.md` — user-facing features, keybindings, usage
- `CLAUDE.md` — keybindings table, conventions, project structure
- `CHANGELOG.md` — add entry under the `## Unreleased` section (create it if missing); do not add to released version sections

## Conventions

- TUI modes: `modeBrowse`, `modeDetail`, `modeEdit`, `modeEditDir`, `modeConfirm`, `modeRename`, `modeFind`
- Don't change an existing key binding without explicitly confirming with the user first
- File operations use `getMarkedOrCurrent()` to work on marked items or cursor item
- Clipboard is an in-memory `[]string` of file paths; `clipboardCut` flag distinguishes copy vs cut
- Cut (`x`) marks files for move without deleting; paste (`p`) uses `os.Rename` (with cross-device fallback)
- Delete always requires confirmation via `modeConfirm`
