package main

// QueueTrack holds metadata for a track in the play queue.
type QueueTrack struct {
	Path     string
	Artist   string
	Album    string
	Title    string
	Year     string
	Duration float64
}

// PlayQueue manages an ordered list of tracks with a current-position cursor.
type PlayQueue struct {
	tracks  []QueueTrack
	current int // index of currently playing track
}

// Replace swaps the queue contents and sets current to startAt.
func (q *PlayQueue) Replace(tracks []QueueTrack, startAt int) {
	q.tracks = make([]QueueTrack, len(tracks))
	copy(q.tracks, tracks)
	q.current = startAt
}

// Append adds tracks to the end of the queue. If the queue was empty, current
// is set to 0.
func (q *PlayQueue) Append(tracks []QueueTrack) {
	wasEmpty := len(q.tracks) == 0
	q.tracks = append(q.tracks, tracks...)
	if wasEmpty && len(q.tracks) > 0 {
		q.current = 0
	}
}

// Remove deletes tracks at the given indices and adjusts current intelligently:
// - If the current track is removed, advance to the next surviving track (or
//   clamp to the new end).
// - If tracks before current are removed, shift current down accordingly.
func (q *PlayQueue) Remove(indices map[int]bool) {
	if len(indices) == 0 {
		return
	}

	currentPath := ""
	currentRemoved := false
	if len(q.tracks) > 0 {
		currentPath = q.tracks[q.current].Path
		currentRemoved = indices[q.current]
	}

	newTracks := q.tracks[:0:0]
	for i, t := range q.tracks {
		if !indices[i] {
			newTracks = append(newTracks, t)
		}
	}
	q.tracks = newTracks

	if len(q.tracks) == 0 {
		q.current = 0
		return
	}

	if currentRemoved {
		// Count surviving tracks before the old current index to find
		// the new index of the first track that comes after it.
		survivingBefore := 0
		for i := 0; i < q.current; i++ {
			if !indices[i] {
				survivingBefore++
			}
		}
		// survivingBefore is the new index of the next track after the
		// removed current; clamp to last track if at end.
		if survivingBefore >= len(q.tracks) {
			survivingBefore = len(q.tracks) - 1
		}
		q.current = survivingBefore
	} else {
		// Current track survived; find its new index by path.
		for i, t := range q.tracks {
			if t.Path == currentPath {
				q.current = i
				return
			}
		}
		// Fallback: count surviving tracks before old current.
		survivingBefore := 0
		for i := 0; i < q.current; i++ {
			if !indices[i] {
				survivingBefore++
			}
		}
		q.current = survivingBefore
	}
}

// Advance moves to the next track. Returns false if already at the last track.
func (q *PlayQueue) Advance() bool {
	if len(q.tracks) == 0 || q.current >= len(q.tracks)-1 {
		return false
	}
	q.current++
	return true
}

// Current returns a pointer to the current track, or nil if the queue is empty.
func (q *PlayQueue) Current() *QueueTrack {
	if len(q.tracks) == 0 {
		return nil
	}
	return &q.tracks[q.current]
}

// CurrentIndex returns the index of the current track.
func (q *PlayQueue) CurrentIndex() int {
	return q.current
}

// Len returns the number of tracks in the queue.
func (q *PlayQueue) Len() int {
	return len(q.tracks)
}

// Tracks returns a copy of all tracks in the queue.
func (q *PlayQueue) Tracks() []QueueTrack {
	out := make([]QueueTrack, len(q.tracks))
	copy(out, q.tracks)
	return out
}

// JumpTo sets current to the given index if valid; otherwise it is a no-op.
func (q *PlayQueue) JumpTo(index int) {
	if index >= 0 && index < len(q.tracks) {
		q.current = index
	}
}

// Clear empties the queue.
func (q *PlayQueue) Clear() {
	q.tracks = nil
	q.current = 0
}
