# soundrig

A terminal-based audio swiss army knife — browse, tag, merge, and manipulate audio files from the comfort of your terminal.

## Vision

Most audio file management tools are either heavyweight GUI applications or bare-bones CLI utilities with no interactivity. soundrig fills the gap: a fast, keyboard-driven TUI for everyday audio tasks, with CLI subcommands for scripting and automation.

Think `lazygit` but for audio files.

## Features

### Available now

- **Merge MP3 files** — combine a directory of MP3s into a single file with proper VBR headers for accurate seeking and duration
- **Auto-tagging** — automatically set ID3 tags (artist, album, title, year) from structured filenames
- **Tag browser** — TUI for browsing ID3 tags across a directory of audio files

### Planned

- **Tag editing** — edit ID3 tags inline from the TUI
- **Batch tag operations** — apply tags across multiple files at once
- **Format conversion** — transcode between MP3, FLAC, OGG, WAV, and other formats
- **Audio splitting** — split files by silence detection, chapter markers, or fixed intervals
- **Normalization** — loudness normalization (ReplayGain / EBU R128)
- **Waveform preview** — visualize audio waveforms in the terminal
- **File renaming** — rename files based on tag metadata (and vice versa)
- **Metadata cleanup** — strip or repair broken tags, embedded artwork management

## Usage

```
soundrig <command> [options]

Commands:
  merge   Merge MP3 files in a directory into a single file
  tags    Browse audio file tags in a TUI
```

### Merge

```
soundrig merge <directory>
```

Merges all MP3 files in `<directory>` (sorted alphabetically) into a single output file. The output filename is derived from the directory name. ID3 tags are set automatically if the filename matches the pattern `YYYY-MM-DD_author_title.mp3`.

### Tags

```
soundrig tags [directory]
```

Opens a TUI to browse ID3 tags for all audio files in the directory (defaults to current directory).

## Building

```
go build -o soundrig .
```

## Tech

- [Go](https://go.dev)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [id3v2](https://github.com/bogem/id3v2) — ID3 tag reading/writing
- [mp3lib](https://github.com/dmulholl/mp3lib) — MP3 frame-level processing
