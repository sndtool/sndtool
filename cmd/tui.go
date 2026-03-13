package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	sndtool "github.com/sndtool/sndtool"
)

const (
	modeBrowse  = ""
	modeDetail  = "detail"
	modeEdit    = "edit"
	modeEditDir = "editdir"
	modeConfirm = "confirm"
	modeRename  = "rename"
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
	dir      string
	entries  []tagEntry
	cursor   int
	offset   int // first visible row for scrolling
	hscroll  int // horizontal scroll offset (columns)
	width    int // terminal width
	height   int // terminal height
	err      error
	quitting bool

	mode          string
	marked        map[int]bool
	clipboard     []string
	viewEntry     tagEntry
	editFields    []editField
	editCursor    int
	editPaths     []string // files to apply edits to
	statusMsg     string
	confirmAction string
}

func (m tagsModel) Init() tea.Cmd {
	return nil
}

// visibleRows returns how many list rows fit on screen (minus header lines).
func (m tagsModel) visibleRows() int {
	// 3 header lines (title, help, blank) + 1 column heading + 1 bottom padding
	const chrome = 5
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

	case tea.KeyMsg:
		m.statusMsg = ""
		// ctrl+c always quits
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
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
		default:
			return m.updateBrowse(msg)
		}
	}
	return m, nil
}

// --- Browse mode ---

func (m tagsModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
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

	case "e":
		if len(m.entries) > 0 {
			e := m.entries[m.cursor]
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
			for _, idx := range targets {
				m.clipboard = append(m.clipboard, m.entries[idx].path)
			}
			m.statusMsg = fmt.Sprintf("Copied %d item(s)", len(targets))
		}

	case "p":
		if len(m.clipboard) > 0 {
			count, err := m.pasteFiles()
			if err != nil {
				m.statusMsg = "Paste error: " + err.Error()
			} else {
				m.statusMsg = fmt.Sprintf("Pasted %d item(s)", count)
				entries, err := loadTags(m.dir)
				if err == nil {
					m.entries = entries
					m.marked = nil
					m = m.clampScroll()
				}
			}
		}

	case "b":
		if len(m.entries) > 0 && m.entries[m.cursor].isDir {
			e := m.entries[m.cursor]
			outputFile := filepath.Join(m.dir, strings.ToLower(filepath.Base(e.path))+".mp3")
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
					m.entries = entries
					m.marked = nil
					m = m.clampScroll()
				}
			}
		}

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
	m.viewEntry = tagEntry{name: filepath.Base(abs), path: abs, isDir: true}
	m.editFields = []editField{
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

	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeBrowse
		return m, nil

	case tea.KeyEnter:
		err := m.saveTags()
		if err != nil {
			m.statusMsg = "Save error: " + err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("Saved tags for %d file(s)", len(m.editPaths))
		}
		m.mode = modeBrowse
		entries, loadErr := loadTags(m.dir)
		if loadErr == nil {
			m.entries = entries
			m.marked = nil
			m = m.clampScroll()
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

func (m tagsModel) saveTags() error {
	fieldMap := make(map[string]string)
	for _, f := range m.editFields {
		val := string(f.value)
		if val == "<mixed>" {
			continue // don't overwrite mixed values
		}
		fieldMap[f.label] = val
	}

	for _, p := range m.editPaths {
		tag, err := id3v2.Open(p, id3v2.Options{Parse: true})
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(p), err)
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
			return fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		tag.Close()
	}
	return nil
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

func (m tagsModel) pasteFiles() (int, error) {
	count := 0
	for _, src := range m.clipboard {
		info, err := os.Stat(src)
		if err != nil {
			return count, err
		}
		name := filepath.Base(src)
		dst := filepath.Join(m.dir, name)
		dst = uniquePath(dst)
		if info.IsDir() {
			err = copyDir(src, dst)
		} else {
			err = copyFile(src, dst)
		}
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
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

// --- Views ---

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dirStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

func (m tagsModel) View() string {
	if m.quitting {
		return ""
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
	default:
		return m.viewBrowse()
	}
}

func (m tagsModel) viewBrowse() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("sndtool tags — "+m.dir) + "\n")
	b.WriteString(dimStyle.Render("j/k: nav  enter: open  e: edit  r: rename  b: merge  d: del  space: mark  c/p: copy/paste  q: quit") + "\n\n")

	heading := fmt.Sprintf("   %-40s  %-20s  %-30s  %s", "File", "Artist", "Title", "Year")
	b.WriteString(headerStyle.Render(hscrollLine(heading, m.hscroll, m.width)) + "\n")

	vis := m.visibleRows()
	end := m.offset + vis
	if end > len(m.entries) {
		end = len(m.entries)
	}

	for i := m.offset; i < end; i++ {
		e := m.entries[i]
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}

		mark := " "
		if m.marked != nil && m.marked[i] {
			mark = "*"
		}

		var line string
		if e.isDir {
			name := e.name + "/"
			if i != m.cursor {
				style = dirStyle
			}
			line = fmt.Sprintf("%s%s%-40s  %-20s  %-30s  %s",
				cursor, mark, truncate(name, 40), "", "<dir>", "")
		} else {
			line = fmt.Sprintf("%s%s%-40s  %-20s  %-30s  %s",
				cursor, mark,
				truncate(e.name, 40),
				truncate(e.artist, 20),
				truncate(e.title, 30),
				e.year,
			)
		}
		b.WriteString(style.Render(hscrollLine(line, m.hscroll, m.width)) + "\n")
	}

	if len(m.entries) > vis {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n  [%d/%d]", m.cursor+1, len(m.entries))))
	}

	if m.statusMsg != "" {
		b.WriteString("\n" + statusStyle.Render("  "+m.statusMsg))
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
	m.entries = entries
	m.cursor = 0
	m.offset = 0
	m.hscroll = 0
	m.marked = nil
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

	if len(entries) == 0 {
		fmt.Println("No audio files found.")
		return nil
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}

	m := tagsModel{dir: abs, entries: entries}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
