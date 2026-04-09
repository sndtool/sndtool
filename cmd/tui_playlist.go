package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// modePlaylistPicker is a modal overlay for adding tracks to playlists.
const modePlaylistPicker = "playlistpicker"

// openPlaylistPicker sets mode and loads playlists, storing paths to add.
func (m tagsModel) openPlaylistPicker(paths []string) tagsModel {
	m.pickerPaths = paths
	m.pickerCursor = 0
	m.pickerNaming = false
	m.pickerNewName = nil
	m.pickerPlaylists = nil
	if m.db != nil {
		playlists, err := ListPlaylists(m.db, nil)
		if err == nil {
			m.pickerPlaylists = playlists
		}
	}
	m.mode = modePlaylistPicker
	return m
}

func (m tagsModel) updatePlaylistPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pickerNaming {
		return m.updatePickerNaming(msg)
	}

	// Total items: index 0 = "New playlist", 1..n = existing playlists
	total := 1 + len(m.pickerPlaylists)

	switch msg.String() {
	case "esc", "q":
		m.mode = modeBrowse
		return m, nil

	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}

	case "down", "j":
		if m.pickerCursor < total-1 {
			m.pickerCursor++
		}

	case "enter":
		if m.pickerCursor == 0 {
			// Enter naming mode for new playlist
			m.pickerNaming = true
			m.pickerNewName = nil
			return m, nil
		}
		// Add to existing playlist
		pl := m.pickerPlaylists[m.pickerCursor-1]
		if m.db != nil && len(m.pickerPaths) > 0 {
			err := AddToPlaylist(m.db, pl.ID, m.pickerPaths)
			if err != nil {
				m.statusMsg = "Error adding to playlist: " + err.Error()
			} else {
				m.statusMsg = fmt.Sprintf("Added %d track(s) to \"%s\"", len(m.pickerPaths), pl.Name)
			}
		}
		m.mode = modeBrowse
		return m, nil
	}

	return m, nil
}

func (m tagsModel) updatePickerNaming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.pickerNaming = false
		return m, nil

	case tea.KeyEnter:
		name := strings.TrimSpace(string(m.pickerNewName))
		if name == "" {
			m.statusMsg = "Playlist name cannot be empty"
			m.pickerNaming = false
			m.mode = modeBrowse
			return m, nil
		}
		if m.db != nil {
			id, err := CreatePlaylist(m.db, name)
			if err != nil {
				m.statusMsg = "Error creating playlist: " + err.Error()
				m.pickerNaming = false
				m.mode = modeBrowse
				return m, nil
			}
			if len(m.pickerPaths) > 0 {
				if err := AddToPlaylist(m.db, id, m.pickerPaths); err != nil {
					m.statusMsg = "Error adding tracks: " + err.Error()
					m.pickerNaming = false
					m.mode = modeBrowse
					return m, nil
				}
			}
			m.statusMsg = fmt.Sprintf("Created playlist \"%s\" with %d track(s)", name, len(m.pickerPaths))
		}
		m.pickerNaming = false
		m.mode = modeBrowse
		return m, nil

	case tea.KeyBackspace:
		if len(m.pickerNewName) > 0 {
			m.pickerNewName = m.pickerNewName[:len(m.pickerNewName)-1]
		}

	case tea.KeyRunes:
		m.pickerNewName = append(m.pickerNewName, msg.Runes...)
	}

	return m, nil
}

func (m tagsModel) viewPlaylistPicker() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Add to Playlist") + "\n")
	b.WriteString(dimStyle.Render("j/k: nav  enter: select  esc: cancel") + "\n\n")

	if m.pickerNaming {
		name := string(m.pickerNewName)
		b.WriteString("  New playlist name: " + name + lipgloss.NewStyle().Background(lipgloss.Color("7")).Foreground(lipgloss.Color("0")).Render(" ") + "\n")
		b.WriteString("\n" + dimStyle.Render("  enter: create  esc: back"))
		return b.String()
	}

	// Index 0 = "New playlist"
	newStyle := lipgloss.NewStyle()
	newCursor := "  "
	if m.pickerCursor == 0 {
		newStyle = selectedStyle
		newCursor = "> "
	}
	b.WriteString(newStyle.Render(newCursor+"[New playlist]") + "\n")

	for i, pl := range m.pickerPlaylists {
		idx := i + 1
		cur := "  "
		s := lipgloss.NewStyle()
		if m.pickerCursor == idx {
			cur = "> "
			s = selectedStyle
		}
		meta := dimStyle.Render(fmt.Sprintf("  %d tracks", pl.TrackCount))
		b.WriteString(s.Render(cur+pl.Name) + meta + "\n")
	}

	b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("  Adding %d track(s)", len(m.pickerPaths))))

	return b.String()
}
