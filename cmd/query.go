package main

import (
	"sort"
	"strings"
)

// ViewType represents what kind of library view to display.
type ViewType string

const (
	ViewMixed    ViewType = ""
	ViewArtist   ViewType = "artist"
	ViewAlbum    ViewType = "album"
	ViewYear     ViewType = "year"
	ViewGenre    ViewType = "genre"
	ViewPlaylist ViewType = "playlist"
	ViewTrack    ViewType = "track"
)

// Query holds the parsed result of a user search input.
type Query struct {
	View         ViewType
	Terms        []string            // general filter terms
	FieldFilters map[string][]string // field-specific filters
}

// viewKeywords are words that set the view when they appear first.
var viewKeywords = map[string]ViewType{
	"artist":   ViewArtist,
	"album":    ViewAlbum,
	"year":     ViewYear,
	"genre":    ViewGenre,
	"playlist": ViewPlaylist,
	"track":    ViewTrack,
}

// fieldKeywords are words that begin a field filter group.
var fieldKeywords = map[string]bool{
	"artist": true,
	"album":  true,
	"year":   true,
	"genre":  true,
}

// Keywords returns a sorted list of all keywords for tab completion.
func Keywords() []string {
	seen := map[string]bool{}
	for k := range viewKeywords {
		seen[k] = true
	}
	for k := range fieldKeywords {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ParseQuery parses a query string into a Query.
// Grammar: [view] [terms...] [field:terms...]
func ParseQuery(input string) Query {
	q := Query{}

	input = strings.TrimSpace(input)
	if input == "" {
		return q
	}

	words := strings.Fields(strings.ToLower(input))
	if len(words) == 0 {
		return q
	}

	i := 0

	// First word: check if it is a view keyword.
	viewSet := false
	if vt, ok := viewKeywords[words[0]]; ok {
		q.View = vt
		viewSet = true
		i = 1
	}

	// Now consume the rest of the words.
	// State: we are either collecting general terms or collecting field-filter terms.
	currentField := "" // empty means collecting general terms

	for i < len(words) {
		w := words[i]

		if fieldKeywords[w] {
			// If view is not set yet this is the first word path — not reachable here
			// because if viewSet is false we don't have a view yet. But fieldKeywords
			// overlap with viewKeywords; the view was already consumed above.
			// Start a new field group.
			currentField = w
			i++
			continue
		}

		if !viewSet && currentField == "" {
			// No view set and not in a field: this is a general term.
			q.Terms = append(q.Terms, w)
			i++
			continue
		}

		if viewSet && currentField == "" {
			// View is set, not in a field group: general terms.
			q.Terms = append(q.Terms, w)
			i++
			continue
		}

		// Inside a field group.
		if q.FieldFilters == nil {
			q.FieldFilters = map[string][]string{}
		}
		q.FieldFilters[currentField] = append(q.FieldFilters[currentField], w)
		i++
	}

	return q
}
