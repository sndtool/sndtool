package cmd

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
}

type tagsModel struct {
	entries  []tagEntry
	cursor   int
	err      error
	quitting bool
}

func (m tagsModel) Init() tea.Cmd {
	return nil
}

func (m tagsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func (m tagsModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("soundrig tags") + "\n")
	b.WriteString(dimStyle.Render("j/k: navigate  q: quit") + "\n\n")

	for i, e := range m.entries {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}

		line := fmt.Sprintf("%s%-40s  %-20s  %-30s  %s",
			cursor,
			truncate(e.name, 40),
			truncate(e.artist, 20),
			truncate(e.title, 30),
			e.year,
		)
		b.WriteString(style.Render(line) + "\n")
	}

	return b.String()
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

	var tags []tagEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".mp3") {
			continue
		}

		path := filepath.Join(dir, name)
		tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
		if err != nil {
			tags = append(tags, tagEntry{path: path, name: name})
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
		tags = append(tags, entry)
	}

	return tags, nil
}

func runTags(args []string) error {
	fs := flag.NewFlagSet("tags", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: soundrig tags <directory>\n\n")
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

	m := tagsModel{entries: entries}
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}
