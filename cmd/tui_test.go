package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(t *testing.T) tagsModel {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	tracks := []TrackRecord{
		{Path: "/music/smith/sermons2024/hope.mp3", Artist: "Smith", Album: "Sermons 2024", Title: "Hope", Year: "2024", Mtime: 1, Duration: 120},
		{Path: "/music/smith/sermons2024/faith.mp3", Artist: "Smith", Album: "Sermons 2024", Title: "Faith", Year: "2024", Mtime: 1, Duration: 90},
		{Path: "/music/jones/hymns/grace.mp3", Artist: "Jones", Album: "Hymns", Title: "Grace", Year: "2025", Mtime: 1, Duration: 180},
	}
	for _, tr := range tracks {
		UpsertTrack(db, tr)
	}

	return tagsModel{
		db:       db,
		hasDB:    true,
		viewMode: viewFiles,
		mode:     modeBrowse,
		queue:    &PlayQueue{},
		width:    80,
		height:   24,
		startDir: "/music",
		dir:      "/music",
	}
}

func sendKey(m tagsModel, key string) tagsModel {
	var msg tea.Msg
	if len(key) == 1 {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	} else {
		switch key {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
	}
	newM, _ := m.Update(msg)
	return newM.(tagsModel)
}

// TestModeCycling verifies tab cycles Library→Queue→Files→Library (with DB).
func TestModeCycling(t *testing.T) {
	m := newTestModel(t)

	if m.viewMode != viewFiles {
		t.Fatalf("initial viewMode = %q, want %q", m.viewMode, viewFiles)
	}

	// Files → Library
	m = sendKey(m, "tab")
	if m.viewMode != viewLibrary {
		t.Errorf("after 1st tab: viewMode = %q, want %q", m.viewMode, viewLibrary)
	}

	// Library → Queue
	m = sendKey(m, "tab")
	if m.viewMode != viewQueue {
		t.Errorf("after 2nd tab: viewMode = %q, want %q", m.viewMode, viewQueue)
	}

	// Queue → Files
	m = sendKey(m, "tab")
	if m.viewMode != viewFiles {
		t.Errorf("after 3rd tab: viewMode = %q, want %q", m.viewMode, viewFiles)
	}
}

// TestModeCycling_ShiftTab verifies shift+tab cycles in reverse: Files→Queue→Library→Files.
func TestModeCycling_ShiftTab(t *testing.T) {
	m := newTestModel(t)

	if m.viewMode != viewFiles {
		t.Fatalf("initial viewMode = %q, want %q", m.viewMode, viewFiles)
	}

	// Files → Queue (reverse)
	m = sendKey(m, "shift+tab")
	if m.viewMode != viewQueue {
		t.Errorf("after 1st shift+tab: viewMode = %q, want %q", m.viewMode, viewQueue)
	}

	// Queue → Library (reverse)
	m = sendKey(m, "shift+tab")
	if m.viewMode != viewLibrary {
		t.Errorf("after 2nd shift+tab: viewMode = %q, want %q", m.viewMode, viewLibrary)
	}

	// Library → Files (reverse)
	m = sendKey(m, "shift+tab")
	if m.viewMode != viewFiles {
		t.Errorf("after 3rd shift+tab: viewMode = %q, want %q", m.viewMode, viewFiles)
	}
}

// TestModeCycling_NoDB verifies that without a DB, tab toggles Files↔Queue (skips library).
func TestModeCycling_NoDB(t *testing.T) {
	m := newTestModel(t)
	m.db = nil
	m.hasDB = false

	if m.viewMode != viewFiles {
		t.Fatalf("initial viewMode = %q, want %q", m.viewMode, viewFiles)
	}

	m = sendKey(m, "tab")
	if m.viewMode != viewQueue {
		t.Errorf("after 1st tab (no DB): viewMode = %q, want %q", m.viewMode, viewQueue)
	}

	m = sendKey(m, "tab")
	if m.viewMode != viewFiles {
		t.Errorf("after 2nd tab (no DB): viewMode = %q, want %q", m.viewMode, viewFiles)
	}

	// Shift+tab reverse without DB
	m = sendKey(m, "shift+tab")
	if m.viewMode != viewQueue {
		t.Errorf("shift+tab from files (no DB): viewMode = %q, want %q", m.viewMode, viewQueue)
	}

	m = sendKey(m, "shift+tab")
	if m.viewMode != viewFiles {
		t.Errorf("shift+tab from queue (no DB): viewMode = %q, want %q", m.viewMode, viewFiles)
	}
}

// TestLibraryQuery_Albums verifies that querying "album" returns 2 albums.
func TestLibraryQuery_Albums(t *testing.T) {
	m := newTestModel(t)
	m.viewMode = viewLibrary
	m.libQuery = "album"
	m.executeLibraryQuery()

	if len(m.libResults) != 2 {
		t.Errorf("libResults len = %d, want 2", len(m.libResults))
	}
	for _, r := range m.libResults {
		if r.entryType != libEntryAlbum {
			t.Errorf("unexpected entry type %v, want libEntryAlbum", r.entryType)
		}
	}
}

// TestLibraryQuery_MixedSearch verifies that querying "smith" returns artist and track entries.
// Note: QueryAlbums filters by album name column only, so "smith" won't match album names
// ("Sermons 2024", "Hymns") — only artist and track results are expected.
func TestLibraryQuery_MixedSearch(t *testing.T) {
	m := newTestModel(t)
	m.viewMode = viewLibrary
	m.libQuery = "smith"
	m.executeLibraryQuery()

	types := map[libEntryType]bool{}
	for _, r := range m.libResults {
		types[r.entryType] = true
	}

	if !types[libEntryArtist] {
		t.Error("expected artist entry in mixed search results for 'smith'")
	}
	if !types[libEntryTrack] {
		t.Error("expected track entry in mixed search results for 'smith'")
	}

	// Verify all Smith tracks appear
	trackCount := 0
	for _, r := range m.libResults {
		if r.entryType == libEntryTrack && r.artist == "Smith" {
			trackCount++
		}
	}
	if trackCount != 2 {
		t.Errorf("expected 2 Smith tracks in results, got %d", trackCount)
	}
}

// TestLibraryQuery_FilteredAlbums verifies that "album sermon" only returns matching albums.
func TestLibraryQuery_FilteredAlbums(t *testing.T) {
	m := newTestModel(t)
	m.viewMode = viewLibrary
	m.libQuery = "album sermon"
	m.executeLibraryQuery()

	if len(m.libResults) == 0 {
		t.Fatal("expected at least one album result for 'album sermon'")
	}
	for _, r := range m.libResults {
		if r.entryType != libEntryAlbum {
			t.Errorf("unexpected entry type %v in filtered album results", r.entryType)
		}
	}
	// Should only match "Sermons 2024", not "Hymns"
	for _, r := range m.libResults {
		if r.label == "Hymns" {
			t.Errorf("unexpected 'Hymns' album in filtered results for 'album sermon'")
		}
	}
}

// TestQueueReplace verifies queue.Replace sets Len and Current correctly.
func TestQueueReplace(t *testing.T) {
	q := &PlayQueue{}

	tracks := []QueueTrack{
		{Path: "/music/a.mp3", Title: "A"},
		{Path: "/music/b.mp3", Title: "B"},
		{Path: "/music/c.mp3", Title: "C"},
	}
	q.Replace(tracks, 1)

	if q.Len() != 3 {
		t.Errorf("Len = %d, want 3", q.Len())
	}
	cur := q.Current()
	if cur == nil {
		t.Fatal("Current() = nil, want track at index 1")
	}
	if cur.Path != "/music/b.mp3" {
		t.Errorf("Current().Path = %q, want %q", cur.Path, "/music/b.mp3")
	}
}

// TestQueueAppend verifies queue.Append adds items and sets current on first append.
func TestQueueAppend(t *testing.T) {
	q := &PlayQueue{}

	first := []QueueTrack{
		{Path: "/music/x.mp3", Title: "X"},
	}
	q.Append(first)

	if q.Len() != 1 {
		t.Errorf("after first Append: Len = %d, want 1", q.Len())
	}
	if q.CurrentIndex() != 0 {
		t.Errorf("after first Append: CurrentIndex = %d, want 0", q.CurrentIndex())
	}

	second := []QueueTrack{
		{Path: "/music/y.mp3", Title: "Y"},
		{Path: "/music/z.mp3", Title: "Z"},
	}
	q.Append(second)

	if q.Len() != 3 {
		t.Errorf("after second Append: Len = %d, want 3", q.Len())
	}
	// current should remain at 0 since queue was not empty
	if q.CurrentIndex() != 0 {
		t.Errorf("after second Append: CurrentIndex = %d, want 0", q.CurrentIndex())
	}
}

// TestLibraryEditingTyping verifies that pressing ':' enters editing mode
// and subsequent character keys are inserted into the query input.
func TestLibraryEditingTyping(t *testing.T) {
	m := newTestModel(t)
	m.viewMode = viewLibrary
	m.libQuery = "album"
	m.executeLibraryQuery()

	// Press ':' to enter editing mode
	m = sendKey(m, ":")
	if !m.libEditing {
		t.Fatal("expected libEditing=true after pressing ':'")
	}
	if string(m.libQueryInput) != "album" {
		t.Fatalf("libQueryInput = %q, want %q", string(m.libQueryInput), "album")
	}

	// Type ' ' (using real KeySpace type, as Bubble Tea sends) then 's'
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = m2.(tagsModel)
	if string(m.libQueryInput) != "album " {
		t.Errorf("after space: libQueryInput = %q, want %q", string(m.libQueryInput), "album ")
	}
	m = sendKey(m, "s")
	if string(m.libQueryInput) != "album s" {
		t.Errorf("after 's': libQueryInput = %q, want %q", string(m.libQueryInput), "album s")
	}
	if !m.libEditing {
		t.Error("expected libEditing still true after typing")
	}
}
