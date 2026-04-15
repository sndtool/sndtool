package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		input       string
		wantView    ViewType
		wantTerms   []string
		wantFilters map[string][]string
	}{
		{
			input:    "album",
			wantView: ViewAlbum,
		},
		{
			input:     "album sermon",
			wantView:  ViewAlbum,
			wantTerms: []string{"sermon"},
		},
		{
			input:     "album sunday sermon",
			wantView:  ViewAlbum,
			wantTerms: []string{"sunday", "sermon"},
		},
		{
			input:       "album sermon year 2025",
			wantView:    ViewAlbum,
			wantTerms:   []string{"sermon"},
			wantFilters: map[string][]string{"year": {"2025"}},
		},
		{
			input:    "track artist smith david year 2025",
			wantView: ViewTrack,
			wantFilters: map[string][]string{
				"artist": {"smith", "david"},
				"year":   {"2025"},
			},
		},
		{
			input:       "album sunday sermon artist johnson",
			wantView:    ViewAlbum,
			wantTerms:   []string{"sunday", "sermon"},
			wantFilters: map[string][]string{"artist": {"johnson"}},
		},
		{
			input:     "artist smith",
			wantView:  ViewArtist,
			wantTerms: []string{"smith"},
		},
		{
			input:    "playlist",
			wantView: ViewPlaylist,
		},
		{
			input:     "sermon on hope",
			wantView:  ViewMixed,
			wantTerms: []string{"sermon", "on", "hope"},
		},
		{
			input:    "",
			wantView: ViewMixed,
		},
		{
			input:     "year 2025",
			wantView:  ViewYear,
			wantTerms: []string{"2025"},
		},
		{
			input:     "genre gospel",
			wantView:  ViewGenre,
			wantTerms: []string{"gospel"},
		},
		{
			input:    "track",
			wantView: ViewTrack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseQuery(tt.input)

			if got.View != tt.wantView {
				t.Errorf("View: got %q, want %q", got.View, tt.wantView)
			}

			if !reflect.DeepEqual(got.Terms, tt.wantTerms) {
				t.Errorf("Terms: got %v, want %v", got.Terms, tt.wantTerms)
			}

			if !reflect.DeepEqual(got.FieldFilters, tt.wantFilters) {
				t.Errorf("FieldFilters: got %v, want %v", got.FieldFilters, tt.wantFilters)
			}
		})
	}
}

func TestKeywords(t *testing.T) {
	kws := Keywords()

	// Must be sorted
	if !sort.StringsAreSorted(kws) {
		t.Errorf("Keywords() is not sorted: %v", kws)
	}

	// Must include all view and field keywords
	expected := []string{"album", "artist", "genre", "playlist", "track", "year"}
	if !reflect.DeepEqual(kws, expected) {
		t.Errorf("Keywords(): got %v, want %v", kws, expected)
	}
}
