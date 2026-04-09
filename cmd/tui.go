package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bogem/id3v2/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dmulholl/mp3lib"
	sndtool "github.com/sndtool/sndtool"
)

const (
	modeBrowse     = ""
	modeDetail     = "detail"
	modeEdit       = "edit"
	modeEditDir    = "editdir"
	modeConfirm    = "confirm"
	modeRename     = "rename"
	modeSearch     = "search"
	modeFind       = "find"
	modeMpvMissing = "mpvmissing"
)

const (
	viewFiles   = "files"
	viewLibrary = "library"
	viewQueue   = "queue"
)

type tagEntry struct {
	path   string
	name   string
	artist string
	album  string
	title  string
	year   string
	isDir  bool
}

type editField struct {
	label string
	value []rune
	pos   int
}

type tagsModel struct {
	dir        string
	allEntries []tagEntry // unfiltered entries from disk
	entries    []tagEntry // visible entries (filtered by search)
	cursor     int
	offset     int // first visible row for scrolling
	hscroll    int // horizontal scroll offset (columns)
	width      int // terminal width
	height     int // terminal height
	err        error
	quitting   bool

	mode          string
	marked        map[int]bool
	clipboard     []string
	clipboardCut  bool
	viewEntry     tagEntry
	editFields    []editField
	editCursor    int
	editPaths     []string // files to apply edits to
	statusMsg     string
	confirmAction string

	searchQuery string // active search term (lowercase)
	searchInput []rune // current search input buffer
	searchPos   int    // cursor position in search input

	// Fuzzy finder state
	findInput  []rune // current find input buffer
	findPos    int    // cursor position in find input
	findTitle  string // header title for find view
	findActive bool   // true when displaying find results in browse view
	returnMode string // mode to return to after edit (modeBrowse or modeFind)

	// View mode
	viewMode string // viewFiles, viewLibrary, viewQueue
	hasDB    bool   // true if sndtool.db is available

	// Play queue
	queue        *PlayQueue
	queueCursor  int          // cursor position in queue view
	queueOffset  int          // scroll offset in queue view
	queueMarked  map[int]bool // marked tracks in queue view

	// Database
	db *sql.DB

	// Library mode state
	libQuery       string     // committed query text
	libQueryInput  []rune     // query input buffer while editing
	libQueryPos    int        // cursor position in query input
	libEditing     bool       // true when : prompt is active
	libResults     []libEntry // current display results
	libCursor      int        // cursor in results
	libOffset      int        // scroll offset
	libDrillStack  []libDrill // breadcrumb stack for drill-down
	libCompletions []string   // tab completion suggestions
	libCompIdx     int        // selected completion (-1 = none)

	// Navigation history
	startDir string   // directory where TUI was launched
	dirStack []string // previous directories for 'b' key

	// Playback state
	playingPath  string    // path of currently playing file
	playCmd      *exec.Cmd // mpv process
	playBlink    bool      // toggles for flashing effect
	mpvSocket    string    // path to mpv IPC socket
	playPosition float64   // current position in seconds
	playDuration float64   // total duration in seconds
	playPaused   bool      // true when playback is paused
	playVolume   float64   // current volume percentage
	playGen      int       // generation counter to discard stale playDoneMsg
}

// tickMsg drives the play indicator blink animation.
type tickMsg struct {
	gen int
}

// playDoneMsg is sent when mpv finishes playing.
type playDoneMsg struct{ gen int }

func tickCmd(gen int) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{gen: gen}
	})
}

func (m tagsModel) Init() tea.Cmd {
	if m.db != nil {
		return m.scanCmd()
	}
	return nil
}

// scanDoneMsg is sent when the background scanner finishes.
type scanDoneMsg struct{ stats ScanStats }

func (m tagsModel) scanCmd() tea.Cmd {
	db := m.db
	dir := m.startDir
	return func() tea.Msg {
		stats, _ := ScanDir(db, dir)
		return scanDoneMsg{stats: stats}
	}
}

// visibleRows returns how many list rows fit on screen (minus header/footer lines).
func (m tagsModel) visibleRows() int {
	// Header: title(1) + help keys(2) + blank(1) + column heading(1) = 5
	chrome := 5

	// Footer: account for elements that will be rendered below the list.
	// Pagination indicator (shown when entries > visible rows, but we need
	// to estimate conservatively to avoid chicken-and-egg).
	chrome += 2 // pagination line + blank separator (always reserve to avoid flicker)

	if m.playingPath != "" || m.statusMsg != "" {
		chrome++ // playback status or status message line
	}
	if m.mode == modeSearch || m.searchQuery != "" {
		chrome++ // search/filter bar
	}

	rows := m.height - chrome
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m tagsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m = m.clampScroll()

	case tickMsg:
		if msg.gen != m.playGen || m.playingPath == "" {
			return m, nil // stale tick from a previous playback
		}
		m.playBlink = !m.playBlink
		if m.mpvSocket != "" {
			if pos, err := queryMpvProperty(m.mpvSocket, "time-pos"); err == nil {
				m.playPosition = pos
			}
			if dur, err := queryMpvProperty(m.mpvSocket, "duration"); err == nil {
				m.playDuration = dur
			}
			if vol, err := queryMpvProperty(m.mpvSocket, "volume"); err == nil {
				m.playVolume = vol
			}
		}
		return m, tickCmd(m.playGen)

	case playDoneMsg:
		if msg.gen != m.playGen || m.playingPath == "" {
			return m, nil
		}
		if m.mpvSocket != "" {
			os.Remove(m.mpvSocket)
		}
		m.playingPath = ""
		m.playCmd = nil
		m.playBlink = false
		m.playPaused = false
		m.mpvSocket = ""
		m.playPosition = 0
		m.playDuration = 0

		if m.queue.Advance() {
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}
		// Queue exhausted
		m.stopPlayback()
		m.statusMsg = "Playback finished"

	case scanDoneMsg:
		m.statusMsg = fmt.Sprintf("Scan: %d added, %d updated, %d removed",
			msg.stats.Added, msg.stats.Updated, msg.stats.Deleted)
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = ""
		// ctrl+c always quits
		if msg.String() == "ctrl+c" {
			m.stopPlayback()
			if m.db != nil {
				m.db.Close()
			}
			m.quitting = true
			return m, tea.Quit
		}
		// View mode routing when in browse mode
		if m.mode == modeBrowse || m.mode == "" {
			switch m.viewMode {
			case viewLibrary:
				return m.updateLibrary(msg)
			case viewQueue:
				return m.updateQueue(msg)
			}
		}
		switch m.mode {
		case modeDetail:
			return m.updateDetail(msg)
		case modeEdit, modeEditDir:
			return m.updateEdit(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modeRename:
			return m.updateRename(msg)
		case modeSearch:
			return m.updateSearch(msg)
		case modeFind:
			return m.updateFind(msg)
		case modeMpvMissing:
			m.mode = modeBrowse
			return m, nil
		default:
			return m.updateBrowse(msg)
		}
	}
	return m, nil
}

// --- Browse mode ---

func (m tagsModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.searchInput = nil
			m.searchPos = 0
			m = m.applyFilter()
			return m, nil
		}
		if m.findActive {
			m.findActive = false
			m.findTitle = ""
			m.findInput = nil
			// Restore directory entries
			entries, err := loadTags(m.dir)
			if err == nil {
				m.allEntries = entries
				m.entries = entries
			}
			m.cursor = 0
			m.offset = 0
			m.marked = nil
			return m, nil
		}
		m.stopPlayback()
		if m.db != nil {
			m.db.Close()
		}
		m.quitting = true
		return m, tea.Quit

	case "q":
		m.stopPlayback()
		if m.db != nil {
			m.db.Close()
		}
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m = m.clampScroll()
		}

	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m = m.clampScroll()
		}

	case "enter":
		if len(m.entries) > 0 {
			e := m.entries[m.cursor]
			if m.findActive {
				// Navigate to the file's directory in browse mode
				var targetDir string
				if e.isDir {
					targetDir = e.path
				} else {
					targetDir = filepath.Dir(e.path)
				}
				m.dirStack = append(m.dirStack, m.dir)
				m.findActive = false
				m.findTitle = ""
				m.findInput = nil
				model, cmd := m.enterDir(targetDir)
				mm := model.(tagsModel)
				targetName := filepath.Base(e.path)
				for i, entry := range mm.entries {
					if entry.name == targetName {
						mm.cursor = i
						mm = mm.clampScroll()
						break
					}
				}
				return mm, cmd
			}
			if e.isDir {
				return m.enterDir(e.path)
			}
			// Show tag detail for files
			m.viewEntry = e
			m.mode = modeDetail
		}

	case "l":
		if len(m.entries) > 0 && m.entries[m.cursor].isDir {
			return m.enterDir(m.entries[m.cursor].path)
		}

	case "right":
		if len(m.entries) > 0 && m.entries[m.cursor].isDir {
			return m.enterDir(m.entries[m.cursor].path)
		}
		m.hscroll += 10

	case "left":
		if len(m.entries) > 0 && m.entries[m.cursor].isDir {
			prev := filepath.Base(m.dir)
			model, cmd := m.enterDir(filepath.Dir(m.dir))
			mm := model.(tagsModel)
			for i, e := range mm.entries {
				if e.isDir && e.name == prev {
					mm.cursor = i
					mm = mm.clampScroll()
					break
				}
			}
			return mm, cmd
		}
		m.hscroll -= 10
		if m.hscroll < 0 {
			m.hscroll = 0
		}

	case "backspace", "h":
		prev := filepath.Base(m.dir)
		model, cmd := m.enterDir(filepath.Dir(m.dir))
		mm := model.(tagsModel)
		for i, e := range mm.entries {
			if e.isDir && e.name == prev {
				mm.cursor = i
				mm = mm.clampScroll()
				break
			}
		}
		return mm, cmd

	case "~":
		// Go to start directory
		if m.dir != m.startDir {
			m.dirStack = append(m.dirStack, m.dir)
			return m.enterDir(m.startDir)
		}

	case "b":
		// Go back to previous directory before a jump
		if len(m.dirStack) > 0 {
			prev := m.dirStack[len(m.dirStack)-1]
			m.dirStack = m.dirStack[:len(m.dirStack)-1]
			return m.enterDir(prev)
		}

	case "f":
		m.mode = modeFind
		if !m.findActive {
			m.findInput = nil
			m.findPos = 0
			m.findActive = true
			m.findTitle = "Find (recursive)"
			m.allEntries = nil
			m.entries = nil
			m.searchQuery = ""
			m.searchInput = nil
			m.searchPos = 0
			m.cursor = 0
			m.offset = 0
			m.marked = nil
		} else {
			m.findPos = len(m.findInput)
		}

	case "e":
		if len(m.entries) > 0 {
			e := m.entries[m.cursor]
			if m.findActive {
				m.returnMode = modeBrowse // findActive stays true
			}
			if e.isDir {
				return m.startEditDir(e.path)
			}
			m = m.startEditFile(e)
		}

	case "d":
		if len(m.entries) > 0 {
			m.confirmAction = "delete"
			m.mode = modeConfirm
		}

	case " ":
		if len(m.entries) > 0 {
			if m.marked == nil {
				m.marked = make(map[int]bool)
			}
			if m.marked[m.cursor] {
				delete(m.marked, m.cursor)
			} else {
				m.marked[m.cursor] = true
			}
			// advance cursor after toggle
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m = m.clampScroll()
			}
		}

	case "c":
		targets := m.getMarkedOrCurrent()
		if len(targets) > 0 {
			m.clipboard = nil
			m.clipboardCut = false
			for _, idx := range targets {
				m.clipboard = append(m.clipboard, m.entries[idx].path)
			}
			m.statusMsg = fmt.Sprintf("Copied %d item(s)", len(targets))
		}

	case "x":
		targets := m.getMarkedOrCurrent()
		if len(targets) > 0 {
			m.clipboard = nil
			m.clipboardCut = true
			for _, idx := range targets {
				m.clipboard = append(m.clipboard, m.entries[idx].path)
			}
			m.marked = nil
			m.statusMsg = fmt.Sprintf("Cut %d item(s)", len(targets))
		}

	case "p":
		if len(m.clipboard) > 0 {
			isCut := m.clipboardCut
			count, dstPaths, err := m.pasteFiles()
			if err != nil {
				m.statusMsg = "Paste error: " + err.Error()
			} else {
				if isCut {
					m.statusMsg = fmt.Sprintf("Moved %d item(s)", count)
					m.clipboard = nil
					m.clipboardCut = false
				} else {
					m.statusMsg = fmt.Sprintf("Pasted %d item(s)", count)
				}
				entries, err := loadTags(m.dir)
				if err == nil {
					m.allEntries = entries
					m.entries = entries
					m.marked = nil
					// highlight first pasted item
					if len(dstPaths) > 0 {
						firstName := filepath.Base(dstPaths[0])
						for i, e := range m.entries {
							if e.name == firstName {
								m.cursor = i
								break
							}
						}
					}
					m = m.clampScroll()
				}
			}
		}

	case "m":
		if len(m.entries) > 0 && m.entries[m.cursor].isDir {
			e := m.entries[m.cursor]
			outputFile := uniquePath(filepath.Join(m.dir, strings.ToLower(filepath.Base(e.path))+".mp3"))
			// Suppress stdout during merge (MergeMp3Files prints progress)
			oldStdout := os.Stdout
			os.Stdout, _ = os.Open(os.DevNull)
			err := sndtool.MergeMp3Files(e.path, outputFile)
			os.Stdout = oldStdout
			if err != nil {
				m.statusMsg = "Merge error: " + err.Error()
			} else {
				_ = sndtool.AddTags(outputFile)
				m.statusMsg = "Merged to " + filepath.Base(outputFile)
				entries, loadErr := loadTags(m.dir)
				if loadErr == nil {
					m.allEntries = entries
					m.entries = entries
					m.marked = nil
					mergedName := filepath.Base(outputFile)
					for i, e := range m.entries {
						if e.name == mergedName {
							m.cursor = i
							break
						}
					}
					m = m.clampScroll()
				}
			}
		}

	case "pgdown", "ctrl+f":
		vis := m.visibleRows()
		m.cursor += vis
		if m.cursor >= len(m.entries) {
			m.cursor = len(m.entries) - 1
		}
		m = m.clampScroll()

	case "pgup", "ctrl+b":
		vis := m.visibleRows()
		m.cursor -= vis
		if m.cursor < 0 {
			m.cursor = 0
		}
		m = m.clampScroll()

	case "r":
		if len(m.entries) > 0 {
			e := m.entries[m.cursor]
			m.mode = modeRename
			m.editPaths = []string{e.path}
			name := e.name
			m.editFields = []editField{
				{label: "Name", value: []rune(name), pos: len([]rune(name))},
			}
			m.editCursor = 0
		}

	case "v":
		switch m.viewMode {
		case viewFiles:
			if m.hasDB {
				m.viewMode = viewLibrary
			} else {
				m.viewMode = viewQueue
			}
		case viewLibrary:
			m.viewMode = viewQueue
		case viewQueue:
			m.viewMode = viewFiles
		}
		return m, nil

	case "/":
		m.mode = modeSearch
		m.searchInput = []rune(m.searchQuery)
		m.searchPos = len(m.searchInput)

	case "P":
		if len(m.entries) == 0 {
			return m, nil
		}
		e := m.entries[m.cursor]
		if e.isDir || !isPlayable(e.path) {
			return m, nil
		}
		// Build queue from current context
		var tracks []QueueTrack
		startIdx := 0
		for _, entry := range m.entries {
			if entry.isDir || !isPlayable(entry.path) {
				continue
			}
			if entry.path == e.path {
				startIdx = len(tracks)
			}
			tracks = append(tracks, QueueTrack{
				Path: entry.path, Artist: entry.artist,
				Album: entry.album, Title: entry.title,
				Year: entry.year,
			})
		}
		m.queue.Replace(tracks, startIdx)
		return m.startPlayback(e.path)

	case "A":
		var tracks []QueueTrack
		if m.marked != nil && len(m.marked) > 0 {
			for i, entry := range m.entries {
				if m.marked[i] && !entry.isDir && isPlayable(entry.path) {
					tracks = append(tracks, QueueTrack{
						Path: entry.path, Artist: entry.artist,
						Album: entry.album, Title: entry.title,
						Year: entry.year,
					})
				}
			}
		} else {
			for _, entry := range m.entries {
				if !entry.isDir && isPlayable(entry.path) {
					tracks = append(tracks, QueueTrack{
						Path: entry.path, Artist: entry.artist,
						Album: entry.album, Title: entry.title,
						Year: entry.year,
					})
				}
			}
		}
		if len(tracks) == 0 {
			return m, nil
		}
		wasEmpty := m.queue.Len() == 0
		m.queue.Append(tracks)
		m.statusMsg = fmt.Sprintf("Added %d track(s) to queue", len(tracks))
		if wasEmpty {
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}
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
		return m, nil

	case "+", "=":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "add", "volume", 5.0)
		}

	case "-":
		if m.playCmd != nil && m.mpvSocket != "" {
			sendMpvCommand(m.mpvSocket, "add", "volume", -5.0)
		}

	case "shift+down":
		if m.queue.CurrentIndex()+1 < m.queue.Len() {
			m.queue.JumpTo(m.queue.CurrentIndex() + 1)
			track := m.queue.Current()
			return m.startPlayback(track.Path)
		}
		return m, nil

	case "Q":
		m.findActive = true
		m.findTitle = "Quality Check — missing tags"
		m.findInput = nil
		m.findPos = 0
		m.searchQuery = ""
		m.searchInput = nil
		m.searchPos = 0
		results := searchMissingTags(m.startDir)
		m.allEntries = results
		m.entries = results
		m.cursor = 0
		m.offset = 0
		m.marked = nil

	}
	return m, nil
}

// --- Detail mode (view tags) ---

func (m tagsModel) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace", "q":
		m.mode = modeBrowse
	case "e":
		m = m.startEditFile(m.viewEntry)
	}
	return m, nil
}

// --- Edit mode ---

func (m tagsModel) startEditFile(e tagEntry) tagsModel {
	m.viewEntry = e
	m.mode = modeEdit
	m.editPaths = []string{e.path}
	m.editFields = []editField{
		{label: "Name", value: []rune(e.name), pos: len([]rune(e.name))},
		{label: "Artist", value: []rune(e.artist), pos: len([]rune(e.artist))},
		{label: "Album", value: []rune(e.album), pos: len([]rune(e.album))},
		{label: "Title", value: []rune(e.title), pos: len([]rune(e.title))},
		{label: "Year", value: []rune(e.year), pos: len([]rune(e.year))},
	}
	m.editCursor = 0
	return m
}

func (m tagsModel) startEditDir(dir string) (tea.Model, tea.Cmd) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}

	dirEntries, err := os.ReadDir(abs)
	if err != nil {
		m.statusMsg = "Error: " + err.Error()
		return m, nil
	}

	var paths []string
	var artists, albums, years []string

	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".mp3") {
			continue
		}
		p := filepath.Join(abs, name)
		tag, err := id3v2.Open(p, id3v2.Options{Parse: true})
		if err != nil {
			paths = append(paths, p)
			artists = append(artists, "")
			albums = append(albums, "")
			years = append(years, "")
			continue
		}
		paths = append(paths, p)
		artists = append(artists, tag.Artist())
		albums = append(albums, tag.Album())
		years = append(years, tag.Year())
		tag.Close()
	}

	if len(paths) == 0 {
		m.statusMsg = "No MP3 files in directory"
		return m, nil
	}

	m.mode = modeEditDir
	m.editPaths = paths
	dirName := filepath.Base(abs)
	m.viewEntry = tagEntry{name: dirName, path: abs, isDir: true}
	m.editFields = []editField{
		{label: "Name", value: []rune(dirName), pos: len([]rune(dirName))},
		{label: "Artist", value: []rune(commonValue(artists)), pos: len([]rune(commonValue(artists)))},
		{label: "Album", value: []rune(commonValue(albums)), pos: len([]rune(commonValue(albums)))},
		{label: "Year", value: []rune(commonValue(years)), pos: len([]rune(commonValue(years)))},
	}
	m.editCursor = 0
	return m, nil
}

func commonValue(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	first := vals[0]
	for _, v := range vals[1:] {
		if v != first {
			return "<mixed>"
		}
	}
	return first
}

func (m tagsModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.editFields[m.editCursor]

	returnTo := m.returnMode
	if returnTo == "" {
		returnTo = modeBrowse
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.mode = returnTo
		m.returnMode = ""
		return m, nil

	case tea.KeyEnter:
		var err error
		m, err = m.saveTags()
		if err != nil {
			m.statusMsg = "Save error: " + err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("Saved tags for %d file(s)", len(m.editPaths))
		}
		m.mode = returnTo
		m.returnMode = ""
		if m.findActive {
			// Update the edited entry in the find results
			for i, e := range m.entries {
				if e.path == m.viewEntry.path {
					tag, terr := id3v2.Open(e.path, id3v2.Options{Parse: true})
					if terr == nil {
						m.entries[i].artist = tag.Artist()
						m.entries[i].album = tag.Album()
						m.entries[i].title = tag.Title()
						m.entries[i].year = tag.Year()
						tag.Close()
					}
					break
				}
			}
		} else {
			entries, loadErr := loadTags(m.dir)
			if loadErr == nil {
				m.allEntries = entries
				m.entries = entries
				m.marked = nil
				m = m.clampScroll()
			}
		}
		return m, nil

	case tea.KeyUp:
		if m.editCursor > 0 {
			m.editCursor--
		}

	case tea.KeyDown, tea.KeyTab:
		if m.editCursor < len(m.editFields)-1 {
			m.editCursor++
		}

	case tea.KeyShiftTab:
		if m.editCursor > 0 {
			m.editCursor--
		}

	case tea.KeyLeft:
		if f.pos > 0 {
			f.pos--
		}

	case tea.KeyRight:
		if f.pos < len(f.value) {
			f.pos++
		}

	case tea.KeyHome, tea.KeyCtrlA:
		f.pos = 0

	case tea.KeyEnd, tea.KeyCtrlE:
		f.pos = len(f.value)

	case tea.KeyBackspace:
		if f.pos > 0 {
			f.value = append(f.value[:f.pos-1], f.value[f.pos:]...)
			f.pos--
		}

	case tea.KeyDelete:
		if f.pos < len(f.value) {
			f.value = append(f.value[:f.pos], f.value[f.pos+1:]...)
		}

	case tea.KeyCtrlU:
		f.value = f.value[f.pos:]
		f.pos = 0

	case tea.KeyCtrlK:
		f.value = f.value[:f.pos]

	case tea.KeySpace:
		newVal := make([]rune, 0, len(f.value)+1)
		newVal = append(newVal, f.value[:f.pos]...)
		newVal = append(newVal, ' ')
		newVal = append(newVal, f.value[f.pos:]...)
		f.value = newVal
		f.pos++

	case tea.KeyRunes:
		runes := msg.Runes
		if len(runes) > 0 {
			newVal := make([]rune, 0, len(f.value)+len(runes))
			newVal = append(newVal, f.value[:f.pos]...)
			newVal = append(newVal, runes...)
			newVal = append(newVal, f.value[f.pos:]...)
			f.value = newVal
			f.pos += len(runes)
		}
	}

	return m, nil
}

func (m tagsModel) saveTags() (tagsModel, error) {
	fieldMap := make(map[string]string)
	for _, f := range m.editFields {
		val := string(f.value)
		if val == "<mixed>" {
			continue // don't overwrite mixed values
		}
		fieldMap[f.label] = val
	}

	// Handle rename if Name field changed
	if newName, ok := fieldMap["Name"]; ok {
		newName = strings.TrimSpace(newName)
		if newName == "" {
			return m, fmt.Errorf("name cannot be empty")
		}
		if m.mode == modeEditDir {
			// Rename directory
			oldPath := m.viewEntry.path
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			if oldPath != newPath {
				if err := os.Rename(oldPath, newPath); err != nil {
					return m, fmt.Errorf("rename dir: %w", err)
				}
				// Update paths to reflect new directory
				for i, p := range m.editPaths {
					rel, _ := filepath.Rel(oldPath, p)
					m.editPaths[i] = filepath.Join(newPath, rel)
				}
				m.dir = newPath
			}
		} else {
			// Rename file
			oldPath := m.editPaths[0]
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			if oldPath != newPath {
				if err := os.Rename(oldPath, newPath); err != nil {
					return m, fmt.Errorf("rename file: %w", err)
				}
				m.editPaths[0] = newPath
			}
		}
		delete(fieldMap, "Name")
	}

	for _, p := range m.editPaths {
		tag, err := id3v2.Open(p, id3v2.Options{Parse: true})
		if err != nil {
			if errors.Is(err, id3v2.ErrBodyOverflow) {
				// Corrupt tag — reopen without parsing to get originalSize,
				// then delete all frames so Save rewrites cleanly.
				tag, err = id3v2.Open(p, id3v2.Options{Parse: false})
				if err != nil {
					return m, fmt.Errorf("%s: %w", filepath.Base(p), err)
				}
				tag.DeleteAllFrames()
			} else {
				return m, fmt.Errorf("%s: %w", filepath.Base(p), err)
			}
		}
		if v, ok := fieldMap["Artist"]; ok {
			tag.SetArtist(v)
		}
		if v, ok := fieldMap["Album"]; ok {
			tag.SetAlbum(v)
		}
		if v, ok := fieldMap["Title"]; ok {
			tag.SetTitle(v)
		}
		if v, ok := fieldMap["Year"]; ok {
			tag.SetYear(v)
		}
		if err := tag.Save(); err != nil {
			tag.Close()
			return m, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		tag.Close()
	}
	return m, nil
}

// --- Confirm mode ---

func (m tagsModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		switch m.confirmAction {
		case "delete":
			targets := m.getMarkedOrCurrent()
			deleted := 0
			var lastErr string
			for _, idx := range targets {
				e := m.entries[idx]
				var err error
				if e.isDir {
					err = os.RemoveAll(e.path)
				} else {
					err = os.Remove(e.path)
				}
				if err != nil {
					lastErr = err.Error()
				} else {
					deleted++
				}
			}
			if lastErr != "" {
				m.statusMsg = fmt.Sprintf("Deleted %d, error: %s", deleted, lastErr)
			} else {
				m.statusMsg = fmt.Sprintf("Deleted %d item(s)", deleted)
			}
			// Reload directory
			entries, err := loadTags(m.dir)
			if err == nil {
				m.allEntries = entries
				m.entries = entries
				m.marked = nil
				if m.cursor >= len(m.entries) {
					m.cursor = len(m.entries) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
				m = m.clampScroll()
			}
		}
		m.mode = modeBrowse

	case "n", "esc", "q":
		m.mode = modeBrowse
	}
	return m, nil
}

// --- Rename mode ---

func (m tagsModel) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.editFields[0]

	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeBrowse
		return m, nil

	case tea.KeyEnter:
		newName := strings.TrimSpace(string(f.value))
		if newName == "" {
			m.statusMsg = "Name cannot be empty"
			m.mode = modeBrowse
			return m, nil
		}
		oldPath := m.editPaths[0]
		newPath := filepath.Join(filepath.Dir(oldPath), newName)
		if oldPath != newPath {
			if err := os.Rename(oldPath, newPath); err != nil {
				m.statusMsg = "Rename error: " + err.Error()
			} else {
				m.statusMsg = "Renamed to " + newName
			}
		}
		m.mode = modeBrowse
		entries, loadErr := loadTags(m.dir)
		if loadErr == nil {
			m.allEntries = entries
			m.entries = entries
			m.marked = nil
			for i, e := range m.entries {
				if e.name == newName {
					m.cursor = i
					break
				}
			}
			m = m.clampScroll()
		}
		return m, nil

	case tea.KeyLeft:
		if f.pos > 0 {
			f.pos--
		}
	case tea.KeyRight:
		if f.pos < len(f.value) {
			f.pos++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		f.pos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		f.pos = len(f.value)
	case tea.KeyBackspace:
		if f.pos > 0 {
			f.value = append(f.value[:f.pos-1], f.value[f.pos:]...)
			f.pos--
		}
	case tea.KeyDelete:
		if f.pos < len(f.value) {
			f.value = append(f.value[:f.pos], f.value[f.pos+1:]...)
		}
	case tea.KeyCtrlU:
		f.value = f.value[f.pos:]
		f.pos = 0
	case tea.KeyCtrlK:
		f.value = f.value[:f.pos]
	case tea.KeySpace:
		newVal := make([]rune, 0, len(f.value)+1)
		newVal = append(newVal, f.value[:f.pos]...)
		newVal = append(newVal, ' ')
		newVal = append(newVal, f.value[f.pos:]...)
		f.value = newVal
		f.pos++
	case tea.KeyRunes:
		runes := msg.Runes
		if len(runes) > 0 {
			newVal := make([]rune, 0, len(f.value)+len(runes))
			newVal = append(newVal, f.value[:f.pos]...)
			newVal = append(newVal, runes...)
			newVal = append(newVal, f.value[f.pos:]...)
			f.value = newVal
			f.pos += len(runes)
		}
	}
	return m, nil
}

// --- Search mode ---

func (m tagsModel) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchQuery = ""
		m.searchInput = nil
		m.searchPos = 0
		m = m.applyFilter()
		m.mode = modeBrowse
		return m, nil

	case tea.KeyEnter:
		m.searchQuery = strings.ToLower(strings.TrimSpace(string(m.searchInput)))
		m = m.applyFilter()
		m.mode = modeBrowse
		return m, nil

	case tea.KeyLeft:
		if m.searchPos > 0 {
			m.searchPos--
		}
	case tea.KeyRight:
		if m.searchPos < len(m.searchInput) {
			m.searchPos++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		m.searchPos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		m.searchPos = len(m.searchInput)
	case tea.KeyBackspace:
		if m.searchPos > 0 {
			m.searchInput = append(m.searchInput[:m.searchPos-1], m.searchInput[m.searchPos:]...)
			m.searchPos--
		}
	case tea.KeyDelete:
		if m.searchPos < len(m.searchInput) {
			m.searchInput = append(m.searchInput[:m.searchPos], m.searchInput[m.searchPos+1:]...)
		}
	case tea.KeyCtrlU:
		m.searchInput = m.searchInput[m.searchPos:]
		m.searchPos = 0
	case tea.KeySpace:
		newVal := make([]rune, 0, len(m.searchInput)+1)
		newVal = append(newVal, m.searchInput[:m.searchPos]...)
		newVal = append(newVal, ' ')
		newVal = append(newVal, m.searchInput[m.searchPos:]...)
		m.searchInput = newVal
		m.searchPos++
	case tea.KeyRunes:
		runes := msg.Runes
		if len(runes) > 0 {
			newVal := make([]rune, 0, len(m.searchInput)+len(runes))
			newVal = append(newVal, m.searchInput[:m.searchPos]...)
			newVal = append(newVal, runes...)
			newVal = append(newVal, m.searchInput[m.searchPos:]...)
			m.searchInput = newVal
			m.searchPos += len(runes)
		}
	}

	// Live filter as user types
	m.searchQuery = strings.ToLower(strings.TrimSpace(string(m.searchInput)))
	m = m.applyFilter()
	return m, nil
}

func (m tagsModel) entryMatchesSearch(e tagEntry) bool {
	if m.searchQuery == "" {
		return false
	}
	q := m.searchQuery
	if strings.Contains(strings.ToLower(e.name), q) ||
		strings.Contains(strings.ToLower(e.artist), q) ||
		strings.Contains(strings.ToLower(e.album), q) ||
		strings.Contains(strings.ToLower(e.title), q) ||
		strings.Contains(strings.ToLower(e.year), q) {
		return true
	}
	// In find mode, also match against the full path
	if m.findActive {
		return strings.Contains(strings.ToLower(e.path), q)
	}
	return false
}

// applyFilter rebuilds m.entries from m.allEntries based on the current search query.
func (m tagsModel) applyFilter() tagsModel {
	if m.searchQuery == "" {
		m.entries = m.allEntries
	} else {
		filtered := make([]tagEntry, 0, len(m.allEntries))
		for _, e := range m.allEntries {
			if m.entryMatchesSearch(e) {
				filtered = append(filtered, e)
			}
		}
		m.entries = filtered
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.offset = 0
	m.marked = nil
	m = m.clampScroll()
	return m
}

// --- Find mode (recursive fuzzy finder) ---

const maxFindResults = 2000

// searchRecursive walks the directory tree from root and returns entries matching query.
func searchRecursive(root, query string) []tagEntry {
	if query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var results []tagEntry

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if len(results) >= maxFindResults {
			return filepath.SkipAll
		}

		name := d.Name()
		nameLower := strings.ToLower(name)

		if d.IsDir() {
			if strings.Contains(nameLower, q) {
				results = append(results, tagEntry{path: path, name: name, isDir: true})
			}
			return nil
		}

		if !strings.HasSuffix(nameLower, ".mp3") {
			return nil
		}

		// Check filename match
		if strings.Contains(nameLower, q) {
			tag, terr := id3v2.Open(path, id3v2.Options{Parse: true})
			if terr != nil {
				results = append(results, tagEntry{path: path, name: name})
			} else {
				results = append(results, tagEntry{
					path: path, name: name,
					artist: tag.Artist(), album: tag.Album(),
					title: tag.Title(), year: tag.Year(),
				})
				tag.Close()
			}
			return nil
		}

		// Check tags
		tag, terr := id3v2.Open(path, id3v2.Options{Parse: true})
		if terr != nil {
			return nil
		}
		defer tag.Close()
		if strings.Contains(strings.ToLower(tag.Artist()), q) ||
			strings.Contains(strings.ToLower(tag.Album()), q) ||
			strings.Contains(strings.ToLower(tag.Title()), q) ||
			strings.Contains(strings.ToLower(tag.Year()), q) {
			results = append(results, tagEntry{
				path: path, name: name,
				artist: tag.Artist(), album: tag.Album(),
				title: tag.Title(), year: tag.Year(),
			})
		}
		return nil
	})
	return results
}

// searchMissingTags walks the directory tree and returns MP3 files missing artist, album, or title.
func searchMissingTags(root string) []tagEntry {
	var results []tagEntry
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(results) >= maxFindResults {
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".mp3") {
			return nil
		}
		tag, terr := id3v2.Open(path, id3v2.Options{Parse: true})
		if terr != nil {
			// Can't read tags — counts as missing
			results = append(results, tagEntry{path: path, name: d.Name()})
			return nil
		}
		defer tag.Close()
		artist := strings.TrimSpace(tag.Artist())
		album := strings.TrimSpace(tag.Album())
		title := strings.TrimSpace(tag.Title())
		if artist == "" || album == "" || title == "" {
			results = append(results, tagEntry{
				path: path, name: d.Name(),
				artist: tag.Artist(), album: tag.Album(),
				title: tag.Title(), year: tag.Year(),
			})
		}
		return nil
	})
	return results
}

func (m tagsModel) updateFind(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Exit find mode entirely, restore directory
		m.mode = modeBrowse
		m.findActive = false
		m.findTitle = ""
		m.findInput = nil
		entries, err := loadTags(m.dir)
		if err == nil {
			m.allEntries = entries
			m.entries = entries
		}
		m.cursor = 0
		m.offset = 0
		m.marked = nil
		return m, nil

	case tea.KeyEnter:
		// Accept search results — switch to browse mode with results still displayed
		m.mode = modeBrowse
		return m, nil

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m = m.clampScroll()
		}

	case tea.KeyDown:
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m = m.clampScroll()
		}

	case tea.KeyLeft:
		if m.findPos > 0 {
			m.findPos--
		}
	case tea.KeyRight:
		if m.findPos < len(m.findInput) {
			m.findPos++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		m.findPos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		m.findPos = len(m.findInput)
	case tea.KeyBackspace:
		if m.findPos > 0 {
			m.findInput = append(m.findInput[:m.findPos-1], m.findInput[m.findPos:]...)
			m.findPos--
			m = m.runFind()
		}
	case tea.KeyDelete:
		if m.findPos < len(m.findInput) {
			m.findInput = append(m.findInput[:m.findPos], m.findInput[m.findPos+1:]...)
			m = m.runFind()
		}
	case tea.KeyCtrlU:
		m.findInput = m.findInput[m.findPos:]
		m.findPos = 0
		m = m.runFind()
	case tea.KeySpace:
		newVal := make([]rune, 0, len(m.findInput)+1)
		newVal = append(newVal, m.findInput[:m.findPos]...)
		newVal = append(newVal, ' ')
		newVal = append(newVal, m.findInput[m.findPos:]...)
		m.findInput = newVal
		m.findPos++
		m = m.runFind()
	case tea.KeyRunes:
		runes := msg.Runes
		if len(runes) > 0 {
			newVal := make([]rune, 0, len(m.findInput)+len(runes))
			newVal = append(newVal, m.findInput[:m.findPos]...)
			newVal = append(newVal, runes...)
			newVal = append(newVal, m.findInput[m.findPos:]...)
			m.findInput = newVal
			m.findPos += len(runes)
			m = m.runFind()
		}
	}
	return m, nil
}

func (m tagsModel) runFind() tagsModel {
	query := strings.TrimSpace(string(m.findInput))
	results := searchRecursive(m.startDir, query)
	m.allEntries = results
	m.entries = results
	// Clear any active filter so it doesn't hide find results
	m.searchQuery = ""
	m.searchInput = nil
	m.searchPos = 0
	m.cursor = 0
	m.offset = 0
	m.marked = nil
	return m
}

// highlightText replaces case-insensitive occurrences of query in text with
// highlighted versions, applying baseStyle to non-matching parts.
func highlightText(text, query string, baseStyle, hlStyle lipgloss.Style) string {
	if query == "" {
		return baseStyle.Render(text)
	}
	lower := strings.ToLower(text)
	var b strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], query)
		if idx < 0 {
			b.WriteString(baseStyle.Render(text[pos:]))
			break
		}
		if idx > 0 {
			b.WriteString(baseStyle.Render(text[pos : pos+idx]))
		}
		b.WriteString(hlStyle.Render(text[pos+idx : pos+idx+len(query)]))
		pos += idx + len(query)
	}
	return b.String()
}

// --- Helpers ---

func (m tagsModel) getMarkedOrCurrent() []int {
	if len(m.marked) > 0 {
		var indices []int
		for idx := range m.marked {
			if idx < len(m.entries) {
				indices = append(indices, idx)
			}
		}
		return indices
	}
	if m.cursor < len(m.entries) {
		return []int{m.cursor}
	}
	return nil
}

func (m tagsModel) pasteFiles() (int, []string, error) {
	count := 0
	var dstPaths []string
	for _, src := range m.clipboard {
		info, err := os.Stat(src)
		if err != nil {
			return count, dstPaths, err
		}
		name := filepath.Base(src)
		dst := filepath.Join(m.dir, name)
		dst = uniquePath(dst)
		if m.clipboardCut {
			// Move: try rename first, fall back to copy+delete for cross-device
			err = os.Rename(src, dst)
			if err != nil {
				// cross-device fallback
				if info.IsDir() {
					err = copyDir(src, dst)
				} else {
					err = copyFile(src, dst)
				}
				if err != nil {
					return count, dstPaths, err
				}
				if info.IsDir() {
					err = os.RemoveAll(src)
				} else {
					err = os.Remove(src)
				}
			}
		} else {
			if info.IsDir() {
				err = copyDir(src, dst)
			} else {
				err = copyFile(src, dst)
			}
		}
		if err != nil {
			return count, dstPaths, err
		}
		dstPaths = append(dstPaths, dst)
		count++
	}
	return count, dstPaths, nil
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else {
			if err := copyFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Playback ---

// mp3Duration returns the duration of an MP3 file by counting frames.
func mp3Duration(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var totalSamples int
	var sampleRate int
	for {
		obj := mp3lib.NextObject(f)
		if obj == nil {
			break
		}
		frame, ok := obj.(*mp3lib.MP3Frame)
		if !ok {
			continue
		}
		if mp3lib.IsXingHeader(frame) || mp3lib.IsVbriHeader(frame) {
			continue
		}
		if sampleRate == 0 {
			sampleRate = frame.SamplingRate
		}
		totalSamples += frame.SampleCount
	}
	if sampleRate == 0 {
		return ""
	}
	secs := totalSamples / sampleRate
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// isPlayable returns true if the file has an audio extension that mpv can play.
func isPlayable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".flac", ".ogg", ".opus", ".wav", ".m4a", ".aac", ".wma":
		return true
	}
	return false
}

func (m *tagsModel) stopPlayback() {
	m.playGen++ // invalidate in-flight tickMsg and playDoneMsg
	if m.playCmd != nil && m.playCmd.Process != nil {
		m.playCmd.Process.Kill()
		m.playCmd.Wait()
	}
	if m.mpvSocket != "" {
		os.Remove(m.mpvSocket)
	}
	m.playCmd = nil
	m.playingPath = ""
	m.playBlink = false
	m.playPaused = false
	m.mpvSocket = ""
	m.playPosition = 0
	m.playDuration = 0
}

func (m tagsModel) startPlayback(path string) (tea.Model, tea.Cmd) {
	m.stopPlayback()

	if _, err := exec.LookPath("mpv"); err != nil {
		m.mode = modeMpvMissing
		return m, nil
	}

	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("sndtool-mpv-%d.sock", os.Getpid()))
	os.Remove(socketPath) // clean up any stale socket

	volArg := fmt.Sprintf("--volume=%d", int(m.playVolume))
	if m.playVolume == 0 {
		volArg = "--volume=100"
	}
	cmd := exec.Command("mpv", "--no-video", volArg, "--input-ipc-server="+socketPath, path)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		m.statusMsg = "Play error: " + err.Error()
		return m, nil
	}
	m.playGen++
	m.playCmd = cmd
	m.playingPath = path
	m.playBlink = true
	m.mpvSocket = socketPath
	m.playPosition = 0
	m.playDuration = 0
	m.statusMsg = ""

	gen := m.playGen
	waitCmd := func() tea.Msg {
		cmd.Wait()
		return playDoneMsg{gen: gen}
	}
	return m, tea.Batch(tickCmd(m.playGen), waitCmd)
}

// sendMpvCommand sends a JSON command to mpv's IPC socket (fire-and-forget).
func sendMpvCommand(socketPath string, args ...interface{}) error {
	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
	payload, _ := json.Marshal(map[string]interface{}{"command": args})
	payload = append(payload, '\n')
	_, err = conn.Write(payload)
	return err
}

// queryMpvProperty sends a get_property command to mpv's IPC socket.
func queryMpvProperty(socketPath, property string) (float64, error) {
	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(100 * time.Millisecond))

	cmd := fmt.Sprintf(`{"command": ["get_property", "%s"]}`, property)
	cmd += "\n"
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return 0, err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, err
	}

	var resp struct {
		Data  float64 `json:"data"`
		Error string  `json:"error"`
	}
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return 0, err
	}
	if resp.Error != "success" {
		return 0, fmt.Errorf("mpv: %s", resp.Error)
	}
	return resp.Data, nil
}

func formatDuration(secs float64) string {
	m := int(secs) / 60
	s := int(secs) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func (m tagsModel) renderPlaybackStatus() string {
	// Find the artist and title for the playing entry
	artist := ""
	title := ""
	for _, e := range m.entries {
		if e.path == m.playingPath {
			artist = e.artist
			title = e.title
			break
		}
	}

	name := title
	if name == "" {
		name = filepath.Base(m.playingPath)
	}

	pos := formatDuration(m.playPosition)
	dur := formatDuration(m.playDuration)

	// Build progress bar
	barWidth := 30
	filled := 0
	if m.playDuration > 0 {
		filled = int(m.playPosition / m.playDuration * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	speaker := "🔊"
	if m.playPaused {
		speaker = "⏸ "
	} else if !m.playBlink {
		speaker = "  "
	}

	label := name
	if artist != "" {
		label = artist + " — " + name
	}

	// Truncate label to fit terminal width.
	// Display columns: "  "(2) + speaker(2) + " "(1) + LABEL + "  "(2) + bar(barWidth) + " "(1) + pos + "/"(1) + dur + " "(1) + vol
	vol := fmt.Sprintf("vol:%d%%", int(m.playVolume))
	fixedWidth := 10 + barWidth + len(pos) + len(dur) + len(vol)
	maxLabel := m.width - fixedWidth
	if maxLabel < 10 {
		maxLabel = 10
	}
	labelRunes := []rune(label)
	if len(labelRunes) > maxLabel {
		label = string(labelRunes[:maxLabel-1]) + "…"
	}

	line := fmt.Sprintf("  %s %s  %s %s/%s %s", speaker, label, bar, pos, dur, vol)
	return playStyle.Render(strings.TrimRight(line, " "))
}

func (m tagsModel) viewMpvMissing() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("sndtool — mpv not found") + "\n\n")
	b.WriteString("  mpv is required for audio playback but was not found on your system.\n\n")
	b.WriteString("  Install mpv:\n")
	b.WriteString("    Linux:   sudo apt install mpv  /  sudo pacman -S mpv\n")
	b.WriteString("    macOS:   brew install mpv\n")
	b.WriteString("    Windows: https://mpv.io/installation/\n\n")
	b.WriteString(dimStyle.Render("  Press any key to dismiss"))
	return b.String()
}

// --- Views ---

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dirStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	matchStyle    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0"))
	playStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")) // blue
)

func (m tagsModel) View() string {
	if m.quitting {
		return ""
	}
	// Route to view-mode renderers when in browse mode
	if m.mode == modeBrowse || m.mode == "" {
		switch m.viewMode {
		case viewLibrary:
			return m.viewLibrary()
		case viewQueue:
			return m.viewQueue()
		}
	}
	switch m.mode {
	case modeDetail:
		return m.viewDetail()
	case modeEdit, modeEditDir:
		return m.viewEdit()
	case modeConfirm:
		return m.viewConfirm()
	case modeRename:
		return m.viewRename()
	case modeSearch:
		return m.viewBrowse()
	case modeFind:
		return m.viewBrowse()
	case modeMpvMissing:
		return m.viewMpvMissing()
	default:
		return m.viewBrowse()
	}
}

func (m tagsModel) viewBrowse() string {
	var b strings.Builder
	modeLabel := " [Files]"
	if m.findActive {
		b.WriteString(headerStyle.Render("sndtool — "+m.findTitle) + dimStyle.Render(modeLabel) + "\n")
		b.WriteString(dimStyle.Render("j/k: nav  e: edit  enter: go to  esc: back  P: play  v: view") + "\n\n")
	} else {
		b.WriteString(headerStyle.Render("sndtool tags — "+m.dir) + dimStyle.Render(modeLabel) + "\n")
		helpKeys := "j/k: nav  enter: open  e: edit  r: rename  d: del  /: filter  space: mark  c/x/p: copy/cut/paste\n" +
			"f: find  Q: quality  b: back  ~: home  m: merge  P: play  S: pause  ⇧←→: seek  ⇧↑↓: prev/next  +/-: vol  v: view  q: quit"
		b.WriteString(dimStyle.Render(helpKeys) + "\n\n")
	}

	// Calculate dynamic column widths based on terminal width.
	// Fixed overhead: cursor(2) + mark(1) + 4 column gaps(2 each=8) + year(4) = 15
	const colGap = "  "
	fixedOverhead := 15
	avail := m.width - fixedOverhead
	if avail < 40 {
		avail = 40
	}
	// Distribute: File 35%, Artist 15%, Album 21%, Title 29%
	colFile := avail * 35 / 100
	colArtist := avail * 15 / 100
	colAlbum := avail * 21 / 100
	colTitle := avail - colFile - colArtist - colAlbum

	headFmt := fmt.Sprintf("   %%-%ds%s%%-%ds%s%%-%ds%s%%-%ds%s%%s",
		colFile, colGap, colArtist, colGap, colAlbum, colGap, colTitle, colGap)
	heading := fmt.Sprintf(headFmt, "File", "Artist", "Album", "Title", "Year")
	b.WriteString(headerStyle.Render(hscrollLine(heading, m.hscroll, m.width)) + "\n")

	vis := m.visibleRows()
	end := m.offset + vis
	if end > len(m.entries) {
		end = len(m.entries)
	}

	rowFmt := fmt.Sprintf("%%s%%s%%-%ds%s%%-%ds%s%%-%ds%s%%-%ds%s%%s",
		colFile, colGap, colArtist, colGap, colAlbum, colGap, colTitle, colGap)

	for i := m.offset; i < end; i++ {
		e := m.entries[i]
		isPlaying := m.playingPath != "" && e.path == m.playingPath
		cursor := "  "
		style := lipgloss.NewStyle()
		if isPlaying {
			if m.playBlink {
				cursor = "🔊"
			} else {
				cursor = "  "
			}
			if i == m.cursor {
				style = selectedStyle
			} else {
				style = playStyle
			}
		} else if i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}

		mark := " "
		if m.marked != nil && m.marked[i] {
			mark = "*"
		}

		// In find mode, show relative path from startDir instead of just name
		displayName := e.name
		if m.findActive {
			if rel, err := filepath.Rel(m.startDir, e.path); err == nil {
				displayName = rel
			}
		}

		var line string
		if e.isDir {
			name := displayName + "/"
			if i != m.cursor && !isPlaying {
				style = dirStyle
			}
			line = fmt.Sprintf(rowFmt,
				cursor, mark, truncate(name, colFile), "", "", "<dir>", "")
		} else {
			line = fmt.Sprintf(rowFmt,
				cursor, mark,
				truncate(displayName, colFile),
				truncate(e.artist, colArtist),
				truncate(e.album, colAlbum),
				truncate(e.title, colTitle),
				e.year,
			)
		}

		scrolled := hscrollLine(line, m.hscroll, m.width)
		findQuery := strings.ToLower(strings.TrimSpace(string(m.findInput)))
		if m.searchQuery != "" && m.entryMatchesSearch(e) {
			b.WriteString(highlightText(scrolled, m.searchQuery, style, matchStyle) + "\n")
		} else if m.findActive && findQuery != "" {
			b.WriteString(highlightText(scrolled, findQuery, style, matchStyle) + "\n")
		} else {
			b.WriteString(style.Render(scrolled) + "\n")
		}
	}

	if len(m.entries) > vis {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n  [%d/%d]", m.cursor+1, len(m.entries))))
	}

	if m.playingPath != "" {
		b.WriteString("\n" + m.renderPlaybackStatus())
	} else if m.statusMsg != "" {
		b.WriteString("\n" + statusStyle.Render("  "+m.statusMsg))
	}

	// Search bar
	if m.mode == modeSearch {
		f := string(m.searchInput)
		b.WriteString("\n" + headerStyle.Render("/") + " ")
		if m.searchPos >= len(m.searchInput) {
			b.WriteString(f + "█")
		} else {
			before := string(m.searchInput[:m.searchPos])
			at := string(m.searchInput[m.searchPos : m.searchPos+1])
			after := string(m.searchInput[m.searchPos+1:])
			b.WriteString(before + lipgloss.NewStyle().Reverse(true).Render(at) + after)
		}
	} else if m.searchQuery != "" {
		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("  filter: %s  (%d/%d, /: edit, esc: clear)", m.searchQuery, len(m.entries), len(m.allEntries))))
	}

	// Find input bar
	if m.mode == modeFind {
		f := string(m.findInput)
		b.WriteString("\n" + headerStyle.Render("find: "))
		if m.findPos >= len(m.findInput) {
			b.WriteString(f + "█")
		} else {
			before := string(m.findInput[:m.findPos])
			at := string(m.findInput[m.findPos : m.findPos+1])
			after := string(m.findInput[m.findPos+1:])
			b.WriteString(before + lipgloss.NewStyle().Reverse(true).Render(at) + after)
		}
	} else if m.findActive {
		findQuery := strings.TrimSpace(string(m.findInput))
		if findQuery != "" {
			b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("  find: %s  (%d results, esc: back)", findQuery, len(m.allEntries))))
		} else {
			b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("  %s  (%d results, esc: back)", m.findTitle, len(m.allEntries))))
		}
	}

	return b.String()
}

func (m tagsModel) viewDetail() string {
	var b strings.Builder
	e := m.viewEntry

	b.WriteString(headerStyle.Render("sndtool — Tag Details") + "\n")
	b.WriteString(dimStyle.Render("esc: back  e: edit") + "\n\n")

	label := dimStyle
	b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("File:  "), e.name))
	b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Path:  "), e.path))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Artist:"), e.artist))
	b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Album: "), e.album))
	b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Title: "), e.title))
	b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Year:  "), e.year))
	if dur := mp3Duration(e.path); dur != "" {
		b.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Length:"), dur))
	}

	return b.String()
}

func (m tagsModel) viewEdit() string {
	var b strings.Builder

	title := "Edit Tags: " + m.viewEntry.name
	if m.mode == modeEditDir {
		title = fmt.Sprintf("Edit Tags: %s/ (%d files)", m.viewEntry.name, len(m.editPaths))
	}
	b.WriteString(headerStyle.Render("sndtool — "+title) + "\n")
	b.WriteString(dimStyle.Render("↑/↓: field  enter: save  esc: cancel") + "\n\n")

	for i, f := range m.editFields {
		label := fmt.Sprintf("  %-8s ", f.label+":")
		if i == m.editCursor {
			b.WriteString(selectedStyle.Render(label))
			// render field value with cursor indicator
			if f.pos >= len(f.value) {
				b.WriteString(string(f.value) + "█")
			} else {
				before := string(f.value[:f.pos])
				at := string(f.value[f.pos : f.pos+1])
				after := string(f.value[f.pos+1:])
				b.WriteString(before + lipgloss.NewStyle().Reverse(true).Render(at) + after)
			}
		} else {
			b.WriteString(dimStyle.Render(label))
			b.WriteString(string(f.value))
		}
		b.WriteString("\n")
	}

	if m.mode == modeEdit {
		if dur := mp3Duration(m.viewEntry.path); dur != "" {
			b.WriteString(fmt.Sprintf("\n  %s  %s\n", dimStyle.Render("Length: "), dur))
		}
	}

	return b.String()
}

func (m tagsModel) viewConfirm() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("sndtool — Confirm") + "\n\n")

	targets := m.getMarkedOrCurrent()
	switch m.confirmAction {
	case "delete":
		if len(targets) == 1 {
			b.WriteString(fmt.Sprintf("  Delete %s? (y/n)", m.entries[targets[0]].name))
		} else {
			b.WriteString(fmt.Sprintf("  Delete %d items? (y/n)\n", len(targets)))
			for _, idx := range targets {
				b.WriteString(fmt.Sprintf("    - %s\n", m.entries[idx].name))
			}
		}
	}

	return b.String()
}

func (m tagsModel) viewRename() string {
	var b strings.Builder
	name := filepath.Base(m.editPaths[0])
	b.WriteString(headerStyle.Render("sndtool — Rename: "+name) + "\n")
	b.WriteString(dimStyle.Render("enter: save  esc: cancel") + "\n\n")

	f := m.editFields[0]
	label := fmt.Sprintf("  %-8s ", f.label+":")
	b.WriteString(selectedStyle.Render(label))
	if f.pos >= len(f.value) {
		b.WriteString(string(f.value) + "█")
	} else {
		before := string(f.value[:f.pos])
		at := string(f.value[f.pos : f.pos+1])
		after := string(f.value[f.pos+1:])
		b.WriteString(before + lipgloss.NewStyle().Reverse(true).Render(at) + after)
	}
	b.WriteString("\n")

	return b.String()
}

// --- Scroll/navigation helpers ---

// clampScroll adjusts offset so cursor stays visible.
func (m tagsModel) clampScroll() tagsModel {
	vis := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
	return m
}

// enterDir loads a new directory and resets cursor/scroll.
func (m tagsModel) enterDir(dir string) (tea.Model, tea.Cmd) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	entries, err := loadTags(abs)
	if err != nil {
		// stay where we are on error
		return m, nil
	}
	m.dir = abs
	m.allEntries = entries
	m.entries = entries
	m.cursor = 0
	m.offset = 0
	m.hscroll = 0
	m.marked = nil
	m.searchQuery = ""
	m.searchInput = nil
	m.searchPos = 0
	return m, nil
}

// hscrollLine applies horizontal scroll to a line, dropping the first
// `off` runes and clamping to `width` columns.
func hscrollLine(s string, off, width int) string {
	runes := []rune(s)
	if off > 0 {
		if off >= len(runes) {
			runes = nil
		} else {
			runes = runes[off:]
		}
	}
	if width > 0 && len(runes) > width {
		runes = runes[:width]
	}
	return string(runes)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// --- Data loading ---

func loadTags(dir string) ([]tagEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs []tagEntry
	var files []tagEntry

	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)

		if e.IsDir() {
			dirs = append(dirs, tagEntry{path: path, name: name, isDir: true})
			continue
		}

		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".mp3") {
			continue
		}

		tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
		if err != nil {
			files = append(files, tagEntry{path: path, name: name})
			continue
		}

		entry := tagEntry{
			path:   path,
			name:   name,
			artist: tag.Artist(),
			album:  tag.Album(),
			title:  tag.Title(),
			year:   tag.Year(),
		}
		tag.Close()
		files = append(files, entry)
	}

	// directories first, then files
	return append(dirs, files...), nil
}

// --- Entry point ---

func runTUI(args []string) error {
	fs := flag.NewFlagSet("sndtool", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sndtool [directory]\n\n")
		fmt.Fprintf(os.Stderr, "Browse and edit audio file tags in a TUI.\n")
	}
	fs.Parse(args)

	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	entries, err := loadTags(dir)
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}

	// DB detection and startup prompt (must happen before tea.NewProgram takes over the terminal).
	var db *sql.DB
	initialViewMode := viewFiles
	dbPath := filepath.Join(abs, "sndtool.db")

	if _, statErr := os.Stat(dbPath); statErr == nil {
		// DB already exists — open it.
		db, err = OpenDB(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open sndtool.db: %v\n", err)
		} else {
			initialViewMode = viewLibrary
		}
	} else {
		// No DB — ask user whether to create one.
		fmt.Print("Create library database? (y/n) ")
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "y" || answer == "yes" {
			db, err = OpenDB(dbPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not create sndtool.db: %v\n", err)
			} else {
				initialViewMode = viewLibrary
			}
		}
	}

	m := tagsModel{
		dir:        abs,
		allEntries: entries,
		entries:    entries,
		startDir:   abs,
		viewMode:   initialViewMode,
		hasDB:      db != nil,
		db:         db,
		queue:      &PlayQueue{},
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
