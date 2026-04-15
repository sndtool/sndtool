package main

import (
	"testing"
)

func makeTracks(n int) []QueueTrack {
	tracks := make([]QueueTrack, n)
	paths := []string{"a.mp3", "b.mp3", "c.mp3", "d.mp3", "e.mp3"}
	for i := 0; i < n; i++ {
		tracks[i] = QueueTrack{Path: paths[i], Title: paths[i]}
	}
	return tracks
}

func TestPlayQueue_ReplaceAndAdvance(t *testing.T) {
	var q PlayQueue
	q.Replace(makeTracks(3), 0)

	if q.Len() != 3 {
		t.Fatalf("expected 3 tracks, got %d", q.Len())
	}
	if q.CurrentIndex() != 0 {
		t.Errorf("expected current index 0, got %d", q.CurrentIndex())
	}
	if q.Current().Path != "a.mp3" {
		t.Errorf("expected a.mp3, got %s", q.Current().Path)
	}

	if !q.Advance() {
		t.Error("expected Advance to return true")
	}
	if q.CurrentIndex() != 1 {
		t.Errorf("expected current index 1, got %d", q.CurrentIndex())
	}

	q.Advance()
	if q.CurrentIndex() != 2 {
		t.Errorf("expected current index 2, got %d", q.CurrentIndex())
	}

	// At end, Advance should return false
	if q.Advance() {
		t.Error("expected Advance to return false at end")
	}
	if q.CurrentIndex() != 2 {
		t.Errorf("expected current to stay at 2, got %d", q.CurrentIndex())
	}
}

func TestPlayQueue_ReplaceStartingFrom(t *testing.T) {
	var q PlayQueue
	q.Replace(makeTracks(3), 1)

	if q.CurrentIndex() != 1 {
		t.Errorf("expected current index 1, got %d", q.CurrentIndex())
	}
	if q.Current().Path != "b.mp3" {
		t.Errorf("expected b.mp3, got %s", q.Current().Path)
	}
}

func TestPlayQueue_Append(t *testing.T) {
	var q PlayQueue
	q.Replace(makeTracks(2), 0)
	q.Advance() // current = 1

	extra := []QueueTrack{{Path: "c.mp3"}, {Path: "d.mp3"}}
	q.Append(extra)

	if q.Len() != 4 {
		t.Fatalf("expected 4 tracks, got %d", q.Len())
	}
	// current should stay at 1
	if q.CurrentIndex() != 1 {
		t.Errorf("expected current index 1, got %d", q.CurrentIndex())
	}
	if q.Tracks()[3].Path != "d.mp3" {
		t.Errorf("expected d.mp3 at index 3, got %s", q.Tracks()[3].Path)
	}
}

func TestPlayQueue_AppendEmpty(t *testing.T) {
	var q PlayQueue
	q.Append(makeTracks(2))

	if q.Len() != 2 {
		t.Fatalf("expected 2 tracks, got %d", q.Len())
	}
	if q.CurrentIndex() != 0 {
		t.Errorf("expected current index 0, got %d", q.CurrentIndex())
	}
}

func TestPlayQueue_Remove(t *testing.T) {
	var q PlayQueue
	q.Replace(makeTracks(4), 2) // current = 2 (c.mp3)

	// Remove track at index 0 (before current)
	q.Remove(map[int]bool{0: true})

	if q.Len() != 3 {
		t.Fatalf("expected 3 tracks, got %d", q.Len())
	}
	// current should have shifted down by 1
	if q.CurrentIndex() != 1 {
		t.Errorf("expected current index 1, got %d", q.CurrentIndex())
	}
	if q.Current().Path != "c.mp3" {
		t.Errorf("expected c.mp3, got %s", q.Current().Path)
	}
}

func TestPlayQueue_RemoveCurrent(t *testing.T) {
	var q PlayQueue
	q.Replace(makeTracks(3), 1) // current = 1 (b.mp3)

	// Remove current track
	q.Remove(map[int]bool{1: true})

	if q.Len() != 2 {
		t.Fatalf("expected 2 tracks, got %d", q.Len())
	}
	// current should advance to what was index 2 (now index 1)
	if q.CurrentIndex() != 1 {
		t.Errorf("expected current index 1, got %d", q.CurrentIndex())
	}
	if q.Current().Path != "c.mp3" {
		t.Errorf("expected c.mp3, got %s", q.Current().Path)
	}
}

func TestPlayQueue_JumpTo(t *testing.T) {
	var q PlayQueue
	q.Replace(makeTracks(3), 0)
	q.JumpTo(2)

	if q.CurrentIndex() != 2 {
		t.Errorf("expected current index 2, got %d", q.CurrentIndex())
	}
	if q.Current().Path != "c.mp3" {
		t.Errorf("expected c.mp3, got %s", q.Current().Path)
	}

	// Out-of-range jump should be a no-op
	q.JumpTo(10)
	if q.CurrentIndex() != 2 {
		t.Errorf("expected current to stay at 2 after invalid jump, got %d", q.CurrentIndex())
	}
}

func TestPlayQueue_Empty(t *testing.T) {
	var q PlayQueue

	if q.Current() != nil {
		t.Error("expected nil current on empty queue")
	}
	if q.Advance() {
		t.Error("expected Advance to return false on empty queue")
	}
	if q.Len() != 0 {
		t.Errorf("expected Len 0, got %d", q.Len())
	}
}
