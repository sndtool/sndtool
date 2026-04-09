package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Library entry types ---

type libEntryType int

const (
	libEntryArtist libEntryType = iota
	libEntryAlbum
	libEntryTrack
	libEntryPlaylist
	libEntryYear
	libEntryGenre
	libEntrySectionHeader
)

type libEntry struct {
	entryType  libEntryType
	label      string  // primary display (artist name, album name, track title)
	sublabel   string  // secondary (e.g., artist for album view)
	count      int     // track or album count
	duration   float64 // total duration
	path       string  // for tracks
	artist     string
	album      string
	title      string
	year       string
	playlistID int64
}

type libDrill struct {
	query      string
	results    []libEntry
	cursor     int
	offset     int
	label      string
	playlistID int64 // non-zero when drilled into a playlist
}

// --- Styles ---

var (
	libKeywordStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	libFieldStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
	libSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	libCursorStyle  = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("7")).Foreground(lipgloss.Color("0"))
)

// --- Update handler ---

func (m tagsModel) updateLibrary(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.libEditing {
			return m.updateLibraryEditing(msg)
		}
		return m.updateLibraryBrowsing(msg)
	}
	return m, nil
}

func (m tagsModel) updateLibraryEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.libQuery = string(m.libQueryInput)
		m.libEditing = false
		m.libCompletions = nil
		m.libCompIdx = -1
		// Add to history if non-empty and different from last entry
		q := strings.TrimSpace(m.libQuery)
		if q != "" && (len(m.libHistory) == 0 || m.libHistory[len(m.libHistory)-1] != q) {
			m.libHistory = append(m.libHistory, q)
		}
		m.libHistoryIdx = -1
		m.executeLibraryQuery()
		return m, nil

	case "esc":
		m.libEditing = false
		m.libQueryInput = []rune(m.libQuery)
		m.libQueryPos = len(m.libQueryInput)
		m.libCompletions = nil
		m.libCompIdx = -1
		return m, nil

	case "backspace":
		if m.libQueryPos > 0 {
			m.libQueryInput = append(m.libQueryInput[:m.libQueryPos-1], m.libQueryInput[m.libQueryPos:]...)
			m.libQueryPos--
			m.updateCompletions()
		}
		return m, nil

	case "left":
		if m.libQueryPos > 0 {
			m.libQueryPos--
		}
		return m, nil

	case "right":
		if m.libQueryPos < len(m.libQueryInput) {
			m.libQueryPos++
		}
		return m, nil

	case "tab":
		if len(m.libCompletions) > 0 {
			idx := m.libCompIdx
			if idx < 0 {
				idx = 0
			}
			m.acceptCompletion(m.libCompletions[idx])
			m.libCompletions = nil
			m.libCompIdx = -1
		}
		return m, nil

	case "up":
		if len(m.libCompletions) > 0 {
			if m.libCompIdx > 0 {
				m.libCompIdx--
			} else {
				m.libCompIdx = len(m.libCompletions) - 1
			}
		} else if len(m.libHistory) > 0 {
			if m.libHistoryIdx < 0 {
				m.libHistoryIdx = len(m.libHistory) - 1
			} else if m.libHistoryIdx > 0 {
				m.libHistoryIdx--
			}
			m.libQueryInput = []rune(m.libHistory[m.libHistoryIdx])
			m.libQueryPos = len(m.libQueryInput)
		}
		return m, nil

	case "down":
		if len(m.libCompletions) > 0 {
			if m.libCompIdx < len(m.libCompletions)-1 {
				m.libCompIdx++
			} else {
				m.libCompIdx = 0
			}
		} else if m.libHistoryIdx >= 0 {
			if m.libHistoryIdx < len(m.libHistory)-1 {
				m.libHistoryIdx++
				m.libQueryInput = []rune(m.libHistory[m.libHistoryIdx])
				m.libQueryPos = len(m.libQueryInput)
			} else {
				// Past end of history — clear to empty
				m.libHistoryIdx = -1
				m.libQueryInput = nil
				m.libQueryPos = 0
			}
		}
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				m.libQueryInput = append(m.libQueryInput[:m.libQueryPos], append([]rune{r}, m.libQueryInput[m.libQueryPos:]...)...)
				m.libQueryPos++
			}
			if len(msg.Runes) > 0 && msg.Runes[0] == ' ' {
				m.libCompletions = nil
				m.libCompIdx = -1
			} else {
				m.updateCompletions()
			}
		}
		return m, nil
	}
}

func (m tagsModel) updateLibraryBrowsing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.libResults)

	switch msg.String() {
	case "q":
		m.stopPlayback()
		m.saveAndCloseDB()
		m.quitting = true
		return m, tea.Quit

	case ":":
		m.libEditing = true
		m.libQueryInput = []rune(m.libQuery)
		m.libQueryPos = len(m.libQueryInput)
		m.libHistoryIdx = -1
		return m, nil

	case "up", "k":
		m.libMoveCursor(-1)
		return m, nil

	case "down", "j":
		m.libMoveCursor(1)
		return m, nil

	case "pgdown", "ctrl+f":
		vis := m.visibleLibraryRows()
		for i := 0; i < vis; i++ {
			m.libMoveCursor(1)
		}
		return m, nil

	case "pgup", "ctrl+b":
		vis := m.visibleLibraryRows()
		for i := 0; i < vis; i++ {
			m.libMoveCursor(-1)
		}
		return m, nil

	case "enter":
		if n == 0 {
			return m, nil
		}
		entry := m.libResults[m.libCursor]
		switch entry.entryType {
		case libEntryTrack:
			return m.libPlayTrack(m.libCursor)
		case libEntrySectionHeader:
			return m, nil
		default:
			m.libDrillInto(entry)
			return m, nil
		}

	case "h", "backspace":
		if len(m.libDrillStack) > 0 {
			top := m.libDrillStack[len(m.libDrillStack)-1]
			m.libDrillStack = m.libDrillStack[:len(m.libDrillStack)-1]
			m.libQuery = top.query
			m.libResults = top.results
			m.libCursor = top.cursor
			m.libOffset = top.offset
		}
		return m, nil

	case "v":
		m.viewMode = viewQueue
		return m, nil

	case "esc":
		if m.libQuery != "" {
			m.libQuery = ""
			m.libResults = nil
			m.libCursor = 0
			m.libOffset = 0
			m.libDrillStack = nil
		}
		return m, nil

	case "P":
		if n == 0 {
			return m, nil
		}
		entry := m.libResults[m.libCursor]
		switch entry.entryType {
		case libEntryTrack:
			return m.libPlayTrack(m.libCursor)
		case libEntryAlbum:
			// Fetch album tracks and play
			tracks, err := QueryTracks(m.db, nil, map[string][]string{"album": {entry.album}})
			if err == nil && len(tracks) > 0 {
				var qt []QueueTrack
				for _, t := range tracks {
					qt = append(qt, QueueTrack{
						Path: t.Path, Artist: t.Artist, Album: t.Album,
						Title: t.Title, Year: t.Year, Duration: t.Duration,
					})
				}
				m.queue.Replace(qt, 0)
				return m.startPlayback(qt[0].Path)
			}
		case libEntryPlaylist:
			// Fetch playlist tracks and play
			tracks, err := GetPlaylistTracks(m.db, entry.playlistID)
			if err == nil && len(tracks) > 0 {
				var qt []QueueTrack
				for _, t := range tracks {
					qt = append(qt, QueueTrack{
						Path: t.Path, Artist: t.Artist, Album: t.Album,
						Title: t.Title, Year: t.Year, Duration: t.Duration,
					})
				}
				m.queue.Replace(qt, 0)
				return m.startPlayback(qt[0].Path)
			}
		case libEntryArtist:
			// Fetch all artist tracks and play
			tracks, err := QueryTracks(m.db, nil, map[string][]string{"artist": {entry.artist}})
			if err == nil && len(tracks) > 0 {
				var qt []QueueTrack
				for _, t := range tracks {
					qt = append(qt, QueueTrack{
						Path: t.Path, Artist: t.Artist, Album: t.Album,
						Title: t.Title, Year: t.Year, Duration: t.Duration,
					})
				}
				m.queue.Replace(qt, 0)
				return m.startPlayback(qt[0].Path)
			}
		}
		return m, nil

	case "A":
		return m.libAppendToQueue()

	case "a":
		if m.db == nil {
			m.statusMsg = "No library database"
			return m, nil
		}
		var paths []string
		// Check marked items first
		if len(m.marked) > 0 {
			for idx, marked := range m.marked {
				if marked && idx < len(m.libResults) {
					e := m.libResults[idx]
					if e.entryType == libEntryTrack && e.path != "" {
						paths = append(paths, e.path)
					} else if (e.entryType == libEntryAlbum) && e.album != "" {
						ff := map[string][]string{"album": {e.album}}
						albumTracks, err := QueryTracks(m.db, nil, ff)
						if err == nil {
							for _, t := range albumTracks {
								paths = append(paths, t.Path)
							}
						}
					}
				}
			}
		} else if n > 0 {
			e := m.libResults[m.libCursor]
			if e.entryType == libEntryTrack && e.path != "" {
				paths = append(paths, e.path)
			} else if e.entryType == libEntryAlbum && e.album != "" {
				ff := map[string][]string{"album": {e.album}}
				albumTracks, err := QueryTracks(m.db, nil, ff)
				if err == nil {
					for _, t := range albumTracks {
						paths = append(paths, t.Path)
					}
				}
			}
		}
		if len(paths) == 0 {
			m.statusMsg = "No tracks to add"
			return m, nil
		}
		m = m.openPlaylistPicker(paths)
		return m, nil

	case "d":
		if n == 0 || m.db == nil {
			return m, nil
		}
		// Check if we're inside a playlist drill-down
		if len(m.libDrillStack) > 0 {
			top := m.libDrillStack[len(m.libDrillStack)-1]
			if top.playlistID != 0 {
				// Remove marked/cursor tracks from this playlist
				var paths []string
				if len(m.marked) > 0 {
					for idx, marked := range m.marked {
						if marked && idx < len(m.libResults) {
							e := m.libResults[idx]
							if e.entryType == libEntryTrack {
								paths = append(paths, e.path)
							}
						}
					}
				} else {
					e := m.libResults[m.libCursor]
					if e.entryType == libEntryTrack {
						paths = append(paths, e.path)
					}
				}
				if len(paths) > 0 {
					if err := RemoveFromPlaylist(m.db, top.playlistID, paths); err != nil {
						m.statusMsg = "Error removing from playlist: " + err.Error()
					} else {
						m.statusMsg = fmt.Sprintf("Removed %d track(s) from playlist", len(paths))
						m.marked = nil
						// Refresh playlist tracks
						tracks, err := GetPlaylistTracks(m.db, top.playlistID)
						if err == nil {
							m.libResults = nil
							for _, t := range tracks {
								m.libResults = append(m.libResults, libEntry{
									entryType: libEntryTrack,
									label:     t.Title,
									sublabel:  t.Artist,
									path:      t.Path,
									artist:    t.Artist,
									album:     t.Album,
									title:     t.Title,
									year:      t.Year,
									duration:  t.Duration,
								})
							}
							if m.libCursor >= len(m.libResults) && m.libCursor > 0 {
								m.libCursor = len(m.libResults) - 1
							}
							m.clampLibraryScroll()
						}
					}
				}
				return m, nil
			}
		}
		// If cursor is on a playlist entry in a playlist view, delete the playlist
		if n > 0 {
			e := m.libResults[m.libCursor]
			if e.entryType == libEntryPlaylist {
				if err := DeletePlaylist(m.db, e.playlistID); err != nil {
					m.statusMsg = "Error deleting playlist: " + err.Error()
				} else {
					m.statusMsg = fmt.Sprintf("Deleted playlist \"%s\"", e.label)
					m.marked = nil
					// Refresh playlist list
					m.libQueryPlaylists(nil)
					if m.libCursor >= len(m.libResults) && m.libCursor > 0 {
						m.libCursor = len(m.libResults) - 1
					}
					m.clampLibraryScroll()
				}
				return m, nil
			}
		}
		return m, nil

	case "r":
		if n == 0 || m.db == nil {
			return m, nil
		}
		e := m.libResults[m.libCursor]
		if e.entryType == libEntryPlaylist {
			m.statusMsg = "Rename: use :playlist to manage playlists (rename not yet supported inline)"
		}
		return m, nil

	case " ":
		if n > 0 && m.libResults[m.libCursor].entryType != libEntrySectionHeader {
			if m.marked == nil {
				m.marked = make(map[int]bool)
			}
			if m.marked[m.libCursor] {
				delete(m.marked, m.libCursor)
			} else {
				m.marked[m.libCursor] = true
			}
			m.libMoveCursor(1)
		}
		return m, nil

	case "S":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "cycle", "pause")
			m.playPaused = !m.playPaused
		}
		return m, nil

	case "shift+right":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "seek", 10.0)
		}
		return m, nil

	case "shift+left":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "seek", -10.0)
		}
		return m, nil

	case "shift+up":
		if m.queue.CurrentIndex() > 0 {
			m.queue.JumpTo(m.queue.CurrentIndex() - 1)
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}
		return m, nil

	case "shift+down":
		if m.queue.CurrentIndex()+1 < m.queue.Len() {
			m.queue.JumpTo(m.queue.CurrentIndex() + 1)
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}
		return m, nil

	case "+", "=":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "add", "volume", 5.0)
		}
		return m, nil

	case "-":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "add", "volume", -5.0)
		}
		return m, nil
	}

	return m, nil
}

// --- Cursor movement ---

func (m *tagsModel) libMoveCursor(dir int) {
	n := len(m.libResults)
	if n == 0 {
		return
	}

	next := m.libCursor + dir
	// Skip section headers
	for next >= 0 && next < n && m.libResults[next].entryType == libEntrySectionHeader {
		next += dir
	}
	if next < 0 {
		next = 0
	}
	if next >= n {
		next = n - 1
	}
	// If we landed on a section header (edge), don't move
	if m.libResults[next].entryType == libEntrySectionHeader {
		return
	}
	m.libCursor = next
	m.clampLibraryScroll()
}

func (m *tagsModel) clampLibraryScroll() {
	vis := m.visibleLibraryRows()
	if m.libCursor < m.libOffset {
		m.libOffset = m.libCursor
	}
	if m.libCursor >= m.libOffset+vis {
		m.libOffset = m.libCursor - vis + 1
	}
}

func (m tagsModel) visibleLibraryRows() int {
	// header(1) + help(1) + blank(1) + query(1) = 4
	// completions take space when editing
	// footer: count(1) + status/playback(1) = 2
	chrome := 6
	compLines := 0
	if m.libEditing && len(m.libCompletions) > 0 {
		compLines = len(m.libCompletions)
		if compLines > 5 {
			compLines = 5
		}
	}
	// breadcrumbs
	if len(m.libDrillStack) > 0 {
		chrome++
	}
	rows := m.height - chrome - compLines
	if rows < 1 {
		rows = 1
	}
	return rows
}

// --- Query execution ---

func (m *tagsModel) executeLibraryQuery() {
	if m.db == nil {
		m.libResults = nil
		return
	}

	q := ParseQuery(m.libQuery)
	m.libCursor = 0
	m.libOffset = 0
	m.marked = nil

	switch q.View {
	case ViewAlbum:
		m.libQueryAlbums(q.Terms)
	case ViewArtist:
		m.libQueryArtists(q.Terms)
	case ViewTrack:
		m.libQueryTracks(q.Terms, q.FieldFilters)
	case ViewPlaylist:
		m.libQueryPlaylists(q.Terms)
	case ViewYear:
		m.libQueryYears(q.Terms)
	case ViewGenre:
		m.libQueryGenres(q.Terms)
	case ViewMixed:
		m.libQueryMixed(q.Terms, q.FieldFilters)
	}

	// Skip initial section headers
	if len(m.libResults) > 0 && m.libResults[0].entryType == libEntrySectionHeader {
		m.libMoveCursor(1)
	}
}

func (m *tagsModel) libQueryAlbums(terms []string) {
	albums, err := QueryAlbums(m.db, terms)
	if err != nil {
		m.statusMsg = "Query error: " + err.Error()
		return
	}
	m.libResults = nil
	for _, a := range albums {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntryAlbum,
			label:     a.Album,
			sublabel:  a.Artist,
			count:     a.TrackCount,
			duration:  a.Duration,
			album:     a.Album,
			artist:    a.Artist,
		})
	}
}

func (m *tagsModel) libQueryArtists(terms []string) {
	artists, err := QueryArtists(m.db, terms)
	if err != nil {
		m.statusMsg = "Query error: " + err.Error()
		return
	}
	m.libResults = nil
	for _, a := range artists {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntryArtist,
			label:     a.Artist,
			count:     a.TrackCount,
			artist:    a.Artist,
		})
	}
}

func (m *tagsModel) libQueryTracks(terms []string, fieldFilters map[string][]string) {
	tracks, err := QueryTracks(m.db, terms, fieldFilters)
	if err != nil {
		m.statusMsg = "Query error: " + err.Error()
		return
	}
	m.libResults = nil
	for _, t := range tracks {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntryTrack,
			label:     t.Title,
			sublabel:  t.Artist,
			path:      t.Path,
			artist:    t.Artist,
			album:     t.Album,
			title:     t.Title,
			year:      t.Year,
			duration:  t.Duration,
		})
	}
}

func (m *tagsModel) libQueryPlaylists(terms []string) {
	playlists, err := ListPlaylists(m.db, terms)
	if err != nil {
		m.statusMsg = "Query error: " + err.Error()
		return
	}
	m.libResults = nil
	for _, p := range playlists {
		m.libResults = append(m.libResults, libEntry{
			entryType:  libEntryPlaylist,
			label:      p.Name,
			count:      p.TrackCount,
			playlistID: p.ID,
		})
	}
}

func (m *tagsModel) libQueryYears(terms []string) {
	years, err := QueryYears(m.db, terms)
	if err != nil {
		m.statusMsg = "Query error: " + err.Error()
		return
	}
	m.libResults = nil
	for _, y := range years {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntryYear,
			label:     y.Year,
			count:     y.TrackCount,
			year:      y.Year,
		})
	}
}

func (m *tagsModel) libQueryGenres(terms []string) {
	genres, err := QueryGenres(m.db, terms)
	if err != nil {
		m.statusMsg = "Query error: " + err.Error()
		return
	}
	m.libResults = nil
	for _, g := range genres {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntryGenre,
			label:     g.Genre,
			count:     g.TrackCount,
		})
	}
}

func (m *tagsModel) libQueryMixed(terms []string, fieldFilters map[string][]string) {
	m.libResults = nil

	// Artists
	artists, _ := QueryArtists(m.db, terms)
	if len(artists) > 0 {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntrySectionHeader,
			label:     "Artists",
		})
		for _, a := range artists {
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryArtist,
				label:     a.Artist,
				count:     a.TrackCount,
				artist:    a.Artist,
			})
		}
	}

	// Albums
	albums, _ := QueryAlbums(m.db, terms)
	if len(albums) > 0 {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntrySectionHeader,
			label:     "Albums",
		})
		for _, a := range albums {
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryAlbum,
				label:     a.Album,
				sublabel:  a.Artist,
				count:     a.TrackCount,
				duration:  a.Duration,
				album:     a.Album,
				artist:    a.Artist,
			})
		}
	}

	// Tracks
	tracks, _ := QueryTracks(m.db, terms, fieldFilters)
	if len(tracks) > 0 {
		m.libResults = append(m.libResults, libEntry{
			entryType: libEntrySectionHeader,
			label:     "Tracks",
		})
		for _, t := range tracks {
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryTrack,
				label:     t.Title,
				sublabel:  t.Artist,
				path:      t.Path,
				artist:    t.Artist,
				album:     t.Album,
				title:     t.Title,
				year:      t.Year,
				duration:  t.Duration,
			})
		}
	}
}

// --- Drill-down ---

func (m *tagsModel) libDrillInto(entry libEntry) {
	if m.db == nil {
		return
	}

	// Save current state
	drill := libDrill{
		query:   m.libQuery,
		results: m.libResults,
		cursor:  m.libCursor,
		offset:  m.libOffset,
		label:   entry.label,
	}
	if entry.entryType == libEntryPlaylist {
		drill.playlistID = entry.playlistID
	}
	m.libDrillStack = append(m.libDrillStack, drill)

	m.libCursor = 0
	m.libOffset = 0
	m.marked = nil

	switch entry.entryType {
	case libEntryArtist:
		// Show albums by this artist
		albums, err := QueryAlbums(m.db, nil)
		if err != nil {
			m.statusMsg = "Query error: " + err.Error()
			return
		}
		m.libResults = nil
		for _, a := range albums {
			if strings.EqualFold(a.Artist, entry.artist) {
				m.libResults = append(m.libResults, libEntry{
					entryType: libEntryAlbum,
					label:     a.Album,
					sublabel:  a.Artist,
					count:     a.TrackCount,
					duration:  a.Duration,
					album:     a.Album,
					artist:    a.Artist,
				})
			}
		}

	case libEntryAlbum:
		// Show tracks in this album
		ff := map[string][]string{"album": {entry.album}}
		tracks, err := QueryTracks(m.db, nil, ff)
		if err != nil {
			m.statusMsg = "Query error: " + err.Error()
			return
		}
		m.libResults = nil
		for _, t := range tracks {
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryTrack,
				label:     t.Title,
				sublabel:  t.Artist,
				path:      t.Path,
				artist:    t.Artist,
				album:     t.Album,
				title:     t.Title,
				year:      t.Year,
				duration:  t.Duration,
			})
		}

	case libEntryPlaylist:
		tracks, err := GetPlaylistTracks(m.db, entry.playlistID)
		if err != nil {
			m.statusMsg = "Query error: " + err.Error()
			return
		}
		m.libResults = nil
		for _, t := range tracks {
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryTrack,
				label:     t.Title,
				sublabel:  t.Artist,
				path:      t.Path,
				artist:    t.Artist,
				album:     t.Album,
				title:     t.Title,
				year:      t.Year,
				duration:  t.Duration,
			})
		}

	case libEntryYear:
		albums, err := QueryAlbumsWithYear(m.db, entry.year)
		if err != nil {
			m.statusMsg = "Query error: " + err.Error()
			return
		}
		m.libResults = nil
		for _, a := range albums {
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryAlbum,
				label:     a.Album,
				sublabel:  a.Artist,
				count:     a.TrackCount,
				duration:  a.Duration,
				album:     a.Album,
				artist:    a.Artist,
			})
		}

	case libEntryGenre:
		ff := map[string][]string{"genre": {entry.label}}
		tracks, err := QueryTracks(m.db, nil, ff)
		if err != nil {
			m.statusMsg = "Query error: " + err.Error()
			return
		}
		// Group by album with section headers
		m.libResults = nil
		currentAlbum := ""
		for _, t := range tracks {
			if t.Album != currentAlbum {
				currentAlbum = t.Album
				m.libResults = append(m.libResults, libEntry{
					entryType: libEntrySectionHeader,
					label:     t.Album,
				})
			}
			m.libResults = append(m.libResults, libEntry{
				entryType: libEntryTrack,
				label:     t.Title,
				sublabel:  t.Artist,
				path:      t.Path,
				artist:    t.Artist,
				album:     t.Album,
				title:     t.Title,
				year:      t.Year,
				duration:  t.Duration,
			})
		}
		// Skip initial section header
		if len(m.libResults) > 0 && m.libResults[0].entryType == libEntrySectionHeader {
			m.libMoveCursor(1)
		}
	}
}

// --- Playback from library ---

func (m tagsModel) libPlayTrack(idx int) (tea.Model, tea.Cmd) {
	// Build queue from all visible tracks, starting at the selected one
	var tracks []QueueTrack
	startAt := 0
	trackIdx := 0
	for i, e := range m.libResults {
		if e.entryType != libEntryTrack {
			continue
		}
		if i == idx {
			startAt = trackIdx
		}
		title := e.title
		if title == "" {
			title = filepath.Base(e.path)
		}
		tracks = append(tracks, QueueTrack{
			Path:     e.path,
			Artist:   e.artist,
			Album:    e.album,
			Title:    title,
			Year:     e.year,
			Duration: e.duration,
		})
		trackIdx++
	}

	if len(tracks) == 0 {
		return m, nil
	}

	m.queue.Replace(tracks, startAt)
	return m.startPlayback(tracks[startAt].Path)
}

func (m tagsModel) libAppendToQueue() (tea.Model, tea.Cmd) {
	var tracks []QueueTrack

	// Check for marked items first
	hasMarked := false
	for idx, marked := range m.marked {
		if marked && idx < len(m.libResults) {
			hasMarked = true
			e := m.libResults[idx]
			if e.entryType == libEntryTrack {
				title := e.title
				if title == "" {
					title = filepath.Base(e.path)
				}
				tracks = append(tracks, QueueTrack{
					Path:     e.path,
					Artist:   e.artist,
					Album:    e.album,
					Title:    title,
					Year:     e.year,
					Duration: e.duration,
				})
			} else if e.entryType == libEntryAlbum && m.db != nil {
				// Fetch album tracks
				ff := map[string][]string{"album": {e.album}}
				albumTracks, err := QueryTracks(m.db, nil, ff)
				if err == nil {
					for _, t := range albumTracks {
						title := t.Title
						if title == "" {
							title = filepath.Base(t.Path)
						}
						tracks = append(tracks, QueueTrack{
							Path:     t.Path,
							Artist:   t.Artist,
							Album:    t.Album,
							Title:    title,
							Year:     t.Year,
							Duration: t.Duration,
						})
					}
				}
			}
		}
	}

	if !hasMarked {
		// Current item
		if len(m.libResults) > 0 {
			e := m.libResults[m.libCursor]
			if e.entryType == libEntryTrack {
				title := e.title
				if title == "" {
					title = filepath.Base(e.path)
				}
				tracks = append(tracks, QueueTrack{
					Path:     e.path,
					Artist:   e.artist,
					Album:    e.album,
					Title:    title,
					Year:     e.year,
					Duration: e.duration,
				})
			} else if e.entryType == libEntryAlbum && m.db != nil {
				ff := map[string][]string{"album": {e.album}}
				albumTracks, err := QueryTracks(m.db, nil, ff)
				if err == nil {
					for _, t := range albumTracks {
						title := t.Title
						if title == "" {
							title = filepath.Base(t.Path)
						}
						tracks = append(tracks, QueueTrack{
							Path:     t.Path,
							Artist:   t.Artist,
							Album:    t.Album,
							Title:    title,
							Year:     t.Year,
							Duration: t.Duration,
						})
					}
				}
			} else {
				// For other group types, append all visible tracks
				for _, re := range m.libResults {
					if re.entryType == libEntryTrack {
						title := re.title
						if title == "" {
							title = filepath.Base(re.path)
						}
						tracks = append(tracks, QueueTrack{
							Path:     re.path,
							Artist:   re.artist,
							Album:    re.album,
							Title:    title,
							Year:     re.year,
							Duration: re.duration,
						})
					}
				}
			}
		}
	}

	if len(tracks) > 0 {
		m.queue.Append(tracks)
		m.marked = nil
		m.statusMsg = fmt.Sprintf("Appended %d track(s) to queue", len(tracks))
	}
	return m, nil
}

// --- Tab completion ---

func (m *tagsModel) updateCompletions() {
	input := string(m.libQueryInput[:m.libQueryPos])
	words := strings.Fields(input)

	// Get the current word being typed
	currentWord := ""
	if len(input) > 0 && input[len(input)-1] != ' ' && len(words) > 0 {
		currentWord = strings.ToLower(words[len(words)-1])
	}

	if currentWord == "" {
		m.libCompletions = nil
		m.libCompIdx = -1
		return
	}

	// Match against keywords
	var matches []string
	for _, kw := range Keywords() {
		if strings.HasPrefix(kw, currentWord) && kw != currentWord {
			matches = append(matches, kw)
		}
	}

	if len(matches) > 10 {
		matches = matches[:10]
	}

	m.libCompletions = matches
	if len(matches) > 0 {
		m.libCompIdx = 0
	} else {
		m.libCompIdx = -1
	}
}

func (m *tagsModel) acceptCompletion(completion string) {
	input := string(m.libQueryInput[:m.libQueryPos])
	words := strings.Fields(input)

	if len(words) == 0 {
		return
	}

	// Replace the last partial word with the completion
	lastWord := words[len(words)-1]
	prefix := input[:len(input)-len(lastWord)]
	newInput := prefix + completion + " "
	m.libQueryInput = []rune(newInput)
	m.libQueryPos = len(m.libQueryInput)
}

// --- View rendering ---

func (m tagsModel) viewLibrary() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  sndtool") + dimStyle.Render("  [Library]") + "\n")
	b.WriteString(dimStyle.Render(":: query  j/k: nav  enter: open  h: back  P: play  A: append  space: mark  S: pause  v: queue  q: quit") + "\n\n")

	// Query line
	b.WriteString(m.renderQueryLine())
	b.WriteString("\n")

	// Completions dropdown
	if m.libEditing && len(m.libCompletions) > 0 {
		max := len(m.libCompletions)
		if max > 5 {
			max = 5
		}
		for i := 0; i < max; i++ {
			if i == m.libCompIdx {
				b.WriteString(selectedStyle.Render("  "+m.libCompletions[i]) + "\n")
			} else {
				b.WriteString(dimStyle.Render("  "+m.libCompletions[i]) + "\n")
			}
		}
	}

	// Breadcrumbs
	if len(m.libDrillStack) > 0 {
		var crumbs []string
		for _, d := range m.libDrillStack {
			lbl := d.label
			if lbl == "" {
				lbl = d.query
			}
			crumbs = append(crumbs, lbl)
		}
		trail := strings.Join(crumbs, " > ")
		b.WriteString(dimStyle.Render("  "+trail) + "\n")
	}

	// Results
	if len(m.libResults) == 0 {
		if m.libQuery == "" {
			b.WriteString(dimStyle.Render("  Type : to search your library.") + "\n")
		} else {
			b.WriteString(dimStyle.Render("  No results.") + "\n")
		}
	} else {
		vis := m.visibleLibraryRows()
		end := m.libOffset + vis
		if end > len(m.libResults) {
			end = len(m.libResults)
		}

		searchTerms := ""
		if m.libQuery != "" {
			q := ParseQuery(m.libQuery)
			searchTerms = strings.Join(q.Terms, " ")
		}

		for i := m.libOffset; i < end; i++ {
			e := m.libResults[i]
			line := m.renderLibEntry(e, i, searchTerms)
			b.WriteString(line + "\n")
		}
	}

	// Footer: count
	b.WriteString(m.renderLibFooter())

	// Playback status
	if m.playingPath != "" {
		b.WriteString("\n" + m.renderPlaybackStatus())
	} else if m.statusMsg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("  "+m.statusMsg))
	}

	return b.String()
}

func (m tagsModel) renderQueryLine() string {
	if m.libEditing {
		// Show : prompt with cursor
		input := string(m.libQueryInput)
		if m.libQueryPos < len(m.libQueryInput) {
			before := string(m.libQueryInput[:m.libQueryPos])
			cursorChar := string(m.libQueryInput[m.libQueryPos])
			after := string(m.libQueryInput[m.libQueryPos+1:])
			return "  :" + before + libCursorStyle.Render(cursorChar) + after
		}
		return "  :" + input + libCursorStyle.Render(" ")
	}

	if m.libQuery == "" {
		return dimStyle.Render("  :")
	}

	// Syntax-highlight the query
	return "  :" + m.highlightQuery(m.libQuery)
}

func (m tagsModel) highlightQuery(query string) string {
	words := strings.Fields(query)
	var parts []string
	for i, w := range words {
		lower := strings.ToLower(w)
		if i == 0 {
			if _, ok := viewKeywords[lower]; ok {
				parts = append(parts, libKeywordStyle.Render(w))
				continue
			}
		}
		if fieldKeywords[lower] {
			parts = append(parts, libFieldStyle.Render(w))
			continue
		}
		parts = append(parts, w)
	}
	return strings.Join(parts, " ")
}

func (m tagsModel) renderLibEntry(e libEntry, idx int, searchTerms string) string {
	if e.entryType == libEntrySectionHeader {
		return libSectionStyle.Render("  -- " + e.label + " --")
	}

	// Cursor and mark indicators
	cursor := "  "
	mark := " "
	if idx == m.libCursor {
		cursor = "> "
	}
	if m.marked != nil && m.marked[idx] {
		mark = "*"
	}

	var style lipgloss.Style
	if idx == m.libCursor {
		style = selectedStyle
	} else {
		style = lipgloss.NewStyle()
	}

	switch e.entryType {
	case libEntryTrack:
		return m.renderTrackEntry(e, cursor, mark, style, searchTerms)
	case libEntryAlbum:
		return m.renderAlbumEntry(e, cursor, mark, style, searchTerms)
	case libEntryArtist:
		return m.renderArtistEntry(e, cursor, mark, style, searchTerms)
	case libEntryPlaylist:
		meta := dimStyle.Render(fmt.Sprintf("  %d tracks", e.count))
		label := e.label
		if searchTerms != "" {
			label = highlightText(label, searchTerms, style, matchStyle)
			return cursor + mark + label + meta
		}
		return style.Render(cursor+mark+label) + meta
	case libEntryYear:
		meta := dimStyle.Render(fmt.Sprintf("  %d tracks", e.count))
		return style.Render(cursor+mark+e.label) + meta
	case libEntryGenre:
		meta := dimStyle.Render(fmt.Sprintf("  %d tracks", e.count))
		label := e.label
		if searchTerms != "" {
			label = highlightText(label, searchTerms, style, matchStyle)
			return cursor + mark + label + meta
		}
		return style.Render(cursor+mark+label) + meta
	}

	return style.Render(cursor + mark + e.label)
}

func (m tagsModel) renderTrackEntry(e libEntry, cursor, mark string, style lipgloss.Style, searchTerms string) string {
	// Calculate column widths
	fixedOverhead := 4 + 6 // cursor+mark + year + gaps
	avail := m.width - fixedOverhead
	if avail < 40 {
		avail = 40
	}
	colArtist := avail * 20 / 100
	colAlbum := avail * 30 / 100
	colTitle := avail * 30 / 100
	colFile := avail - colArtist - colAlbum - colTitle

	artist := truncate(e.artist, colArtist)
	album := truncate(e.album, colAlbum)
	title := e.title
	if title == "" {
		title = filepath.Base(e.path)
	}
	title = truncate(title, colTitle)
	year := e.year
	if len(year) > 4 {
		year = year[:4]
	}
	filename := truncate(filepath.Base(e.path), colFile)

	line := fmt.Sprintf("%s%s%-*s  %-*s  %-*s  %-4s  %s",
		cursor, mark,
		colArtist, artist,
		colAlbum, album,
		colTitle, title,
		year,
		filename)

	if searchTerms != "" {
		return highlightText(line, searchTerms, style, matchStyle)
	}
	return style.Render(line)
}

func (m tagsModel) renderAlbumEntry(e libEntry, cursor, mark string, style lipgloss.Style, searchTerms string) string {
	avail := m.width - 4 // cursor+mark
	if avail < 40 {
		avail = 40
	}
	colAlbum := avail * 40 / 100
	colArtist := avail * 30 / 100

	album := truncate(e.label, colAlbum)
	artist := truncate(e.sublabel, colArtist)
	meta := fmt.Sprintf("%d tracks  %s", e.count, formatDuration(e.duration))

	line := fmt.Sprintf("%s%s%-*s  %-*s  %s",
		cursor, mark,
		colAlbum, album,
		colArtist, artist,
		meta)

	if searchTerms != "" {
		return highlightText(line, searchTerms, style, matchStyle)
	}
	return style.Render(line)
}

func (m tagsModel) renderArtistEntry(e libEntry, cursor, mark string, style lipgloss.Style, searchTerms string) string {
	meta := dimStyle.Render(fmt.Sprintf("  %d tracks", e.count))
	label := e.label
	if searchTerms != "" {
		label = highlightText(label, searchTerms, style, matchStyle)
		return cursor + mark + label + meta
	}
	return style.Render(cursor+mark+label) + meta
}

func (m tagsModel) renderLibFooter() string {
	if len(m.libResults) == 0 {
		return ""
	}

	// Count by type
	counts := map[libEntryType]int{}
	for _, e := range m.libResults {
		if e.entryType != libEntrySectionHeader {
			counts[e.entryType]++
		}
	}

	var parts []string
	if c := counts[libEntryArtist]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d artist(s)", c))
	}
	if c := counts[libEntryAlbum]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d album(s)", c))
	}
	if c := counts[libEntryTrack]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d track(s)", c))
	}
	if c := counts[libEntryPlaylist]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d playlist(s)", c))
	}
	if c := counts[libEntryYear]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d year(s)", c))
	}
	if c := counts[libEntryGenre]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d genre(s)", c))
	}

	summary := strings.Join(parts, ", ")
	return "\n" + dimStyle.Render(fmt.Sprintf("  [%s]  %d/%d", summary, m.libCursor+1, len(m.libResults)))
}
