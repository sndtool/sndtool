package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		// re-clamp offset after resize
		m = m.clampScroll()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
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

		case "enter", "l":
			if len(m.entries) > 0 && m.entries[m.cursor].isDir {
				return m.enterDir(m.entries[m.cursor].path)
			}

		case "right":
			if len(m.entries) > 0 && m.entries[m.cursor].isDir {
				return m.enterDir(m.entries[m.cursor].path)
			}
			const hstep = 10
			m.hscroll += hstep

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
			const hstepLeft = 10
			m.hscroll -= hstepLeft
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
		}
	}
	return m, nil
}

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
	return m, nil
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

var dirStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

func (m tagsModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("sndtool tags — "+m.dir) + "\n")
	b.WriteString(dimStyle.Render("j/k: navigate  enter/l: open dir  backspace/h: parent  ←/→: scroll  q: quit") + "\n\n")

	heading := fmt.Sprintf("  %-40s  %-20s  %-30s  %s", "File", "Artist", "Title", "Year")
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

		var line string
		if e.isDir {
			name := e.name + "/"
			if i == m.cursor {
				style = selectedStyle
			} else {
				style = dirStyle
			}
			line = fmt.Sprintf("%s%-40s  %-20s  %-30s  %s",
				cursor, truncate(name, 40), "", "<dir>", "")
		} else {
			line = fmt.Sprintf("%s%-40s  %-20s  %-30s  %s",
				cursor,
				truncate(e.name, 40),
				truncate(e.artist, 20),
				truncate(e.title, 30),
				e.year,
			)
		}
		b.WriteString(style.Render(hscrollLine(line, m.hscroll, m.width)) + "\n")
	}

	// scroll indicator
	if len(m.entries) > vis {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n  [%d/%d]", m.cursor+1, len(m.entries))))
	}

	return b.String()
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

func runTags(args []string) error {
	fs := flag.NewFlagSet("tags", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sndtool tags <directory>\n\n")
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
