package importer

import (
	"strings"
	"unicode"

	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// matchThreshold is the minimum normalized Levenshtein similarity for a
// fuzzy title match; anything below lands in the UNRESOLVED bucket for
// manual review.
const matchThreshold = 0.6

// normalizeTitle lowercases a title, replaces punctuation with spaces, and
// collapses whitespace so "Breaking Bad!" and "breaking  bad" compare equal.
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// chooseMatch picks a TMDB ID for a title from search results: an exact
// normalized-title match wins; otherwise the highest fuzzy similarity above
// matchThreshold; otherwise 0 (unresolved).
func chooseMatch(title string, results []tmdb.SearchResult) int {
	norm := normalizeTitle(title)
	if norm == "" {
		return 0
	}
	for _, r := range results {
		if normalizeTitle(r.Name) == norm {
			return r.ID
		}
	}
	bestID, bestScore := 0, 0.0
	for _, r := range results {
		if s := similarity(norm, normalizeTitle(r.Name)); s > bestScore {
			bestID, bestScore = r.ID, s
		}
	}
	if bestScore >= matchThreshold {
		return bestID
	}
	return 0
}

// similarity is 1 - levenshtein/maxLen over rune slices, in [0,1].
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	longest := len(ra)
	if len(rb) > longest {
		longest = len(rb)
	}
	if longest == 0 {
		return 0
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longest)
}

// levenshtein computes edit distance with a single rolling row.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := prev[0]
		prev[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			next := min3(prev[j]+1, prev[j-1]+1, cur+cost)
			cur = prev[j]
			prev[j] = next
		}
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
