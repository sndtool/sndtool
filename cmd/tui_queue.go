package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// queueCursor is a separate cursor field for the queue view so it doesn't
// conflict with the file browser cursor.  We store it in tagsModel via a
// dedicated field added to the struct by Task 7; if that field doesn't exist
// yet we use the model's main cursor. We store it as queueCursor int in
// tagsModel — if the field is missing the compiler will catch it.

// updateQueue handles keyboard input when viewMode == viewQueue.
func (m tagsModel) updateQueue(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	tracks := m.queue.Tracks()
	n := len(tracks)

	switch keyMsg.String() {
	case "q":
		m.stopPlayback()
		if m.db != nil {
			m.db.Close()
		}
		m.quitting = true
		return m, tea.Quit

	case "v":
		m.viewMode = viewFiles
		return m, nil

	case "up", "k":
		if m.queueCursor > 0 {
			m.queueCursor--
			m = m.clampQueueScroll()
		}

	case "down", "j":
		if m.queueCursor < n-1 {
			m.queueCursor++
			m = m.clampQueueScroll()
		}

	case " ":
		// Mark/unmark selected queue entry
		if n > 0 {
			if m.queueMarked == nil {
				m.queueMarked = make(map[int]bool)
			}
			if m.queueMarked[m.queueCursor] {
				delete(m.queueMarked, m.queueCursor)
			} else {
				m.queueMarked[m.queueCursor] = true
			}
			if m.queueCursor < n-1 {
				m.queueCursor++
				m = m.clampQueueScroll()
			}
		}

	case "d":
		// Remove marked tracks, or current track if none marked
		if n > 0 {
			toRemove := make(map[int]bool)
			if len(m.queueMarked) > 0 {
				toRemove = m.queueMarked
			} else {
				toRemove[m.queueCursor] = true
			}
			m.queue.Remove(toRemove)
			m.queueMarked = nil
			newLen := m.queue.Len()
			if m.queueCursor >= newLen && m.queueCursor > 0 {
				m.queueCursor = newLen - 1
			}
			m = m.clampQueueScroll()
			m.statusMsg = fmt.Sprintf("Removed %d track(s) from queue", len(toRemove))
		}

	case "P":
		// Jump playback to selected track
		if n > 0 {
			m.queue.JumpTo(m.queueCursor)
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}

	case "a":
		if m.db == nil {
			m.statusMsg = "No library database"
			return m, nil
		}
		var paths []string
		for _, t := range tracks {
			paths = append(paths, t.Path)
		}
		if len(paths) == 0 {
			m.statusMsg = "Queue is empty"
			return m, nil
		}
		m = m.openPlaylistPicker(paths)
		return m, nil

	case "S":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "cycle", "pause")
			m.playPaused = !m.playPaused
		}

	case "shift+right":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "seek", 10.0)
		}

	case "shift+left":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "seek", -10.0)
		}

	case "shift+up":
		if m.queue.CurrentIndex() > 0 {
			m.queue.JumpTo(m.queue.CurrentIndex() - 1)
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}

	case "shift+down":
		if m.queue.CurrentIndex()+1 < m.queue.Len() {
			m.queue.JumpTo(m.queue.CurrentIndex() + 1)
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}

	case "+", "=":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "add", "volume", 5.0)
		}

	case "-":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "add", "volume", -5.0)
		}
	}

	return m, nil
}

// clampQueueScroll adjusts queueOffset so queueCursor stays visible.
func (m tagsModel) clampQueueScroll() tagsModel {
	vis := m.visibleQueueRows()
	if m.queueCursor < m.queueOffset {
		m.queueOffset = m.queueCursor
	}
	if m.queueCursor >= m.queueOffset+vis {
		m.queueOffset = m.queueCursor - vis + 1
	}
	return m
}

// visibleQueueRows returns how many track rows fit in the queue view.
func (m tagsModel) visibleQueueRows() int {
	// Header(1) + help(1) + blank(1) + col heading(1) = 4 chrome
	// Footer: pagination(2) + status(1) = 3
	chrome := 7
	if m.playingPath == "" && m.statusMsg == "" {
		chrome-- // no status line
	}
	rows := m.height - chrome
	if rows < 1 {
		rows = 1
	}
	return rows
}

// viewQueue renders the queue view.
func (m tagsModel) viewQueue() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  sndtool") + dimStyle.Render("  [Queue]") + "\n")
	b.WriteString(dimStyle.Render("j/k: nav  space: mark  d: remove  P: play  S: pause  ⇧←→: seek  ⇧↑↓: prev/next  +/-: vol  v: files  q: quit") + "\n\n")

	tracks := m.queue.Tracks()
	n := len(tracks)

	if n == 0 {
		b.WriteString(dimStyle.Render("  Queue is empty. Press P on a track to start playing.") + "\n")
		if m.statusMsg != "" {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("  "+m.statusMsg))
		}
		return b.String()
	}

	// Column widths: #(3) + gap(2) + artist + gap + album + gap + title
	const colGap = "  "
	fixedOverhead := 3 + 2 + 2 + 2 + 4 // num + gaps + cursor/mark overhead
	avail := m.width - fixedOverhead
	if avail < 40 {
		avail = 40
	}
	colArtist := avail * 25 / 100
	colAlbum := avail * 30 / 100
	colTitle := avail - colArtist - colAlbum

	headFmt := fmt.Sprintf("   %%3s%s%%-%ds%s%%-%ds%s%%s",
		colGap, colArtist, colGap, colAlbum, colGap)
	heading := fmt.Sprintf(headFmt, "#", "Artist", "Album", "Title")
	b.WriteString(headerStyle.Render(heading) + "\n")

	vis := m.visibleQueueRows()
	end := m.queueOffset + vis
	if end > n {
		end = n
	}

	currentIdx := m.queue.CurrentIndex()

	for i := m.queueOffset; i < end; i++ {
		t := tracks[i]

		isPlaying := m.playingPath != "" && t.Path == m.playingPath
		isCurrent := i == currentIdx && m.playingPath != ""

		// Cursor indicator
		cursor := "  "
		var style lipgloss.Style
		if isPlaying || isCurrent {
			if m.playBlink {
				cursor = "🔊"
			} else {
				cursor = "  "
			}
			if i == m.queueCursor {
				style = selectedStyle
			} else {
				style = playStyle
			}
		} else if i == m.queueCursor {
			cursor = "> "
			style = selectedStyle
		} else {
			style = lipgloss.NewStyle()
		}

		mark := " "
		if m.queueMarked != nil && m.queueMarked[i] {
			mark = "*"
		}

		artist := truncate(t.Artist, colArtist)
		album := truncate(t.Album, colAlbum)
		title := truncate(t.Title, colTitle)
		if title == "" {
			title = truncate(filepath.Base(t.Path), colTitle)
		}

		rowFmt := fmt.Sprintf("%%s%%s%%3d%s%%-%ds%s%%-%ds%s%%s",
			colGap, colArtist, colGap, colAlbum, colGap)
		line := fmt.Sprintf(rowFmt, cursor, mark, i+1, artist, album, title)
		b.WriteString(style.Render(line) + "\n")
	}

	if n > vis {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n  [%d/%d]", m.queueCursor+1, n)))
	}

	if m.playingPath != "" {
		b.WriteString("\n" + m.renderPlaybackStatus())
	} else if m.statusMsg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("  "+m.statusMsg))
	}

	return b.String()
}
