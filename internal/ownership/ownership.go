// Package ownership defines the fixed sets of physical formats a user can own
// per media type, and helpers to validate and (de)serialize them.
//
// Ownership is stored on a TrackingItem as a comma-joined string (e.g.
// "Vinyl,CD"). Only media types present in allowedFormats support ownership;
// music is the first (its albums are owned on Vinyl and/or CD). The set is a
// closed vocabulary — an unknown format is rejected so the shelf never records
// a typo.
package ownership

import (
	"errors"
	"fmt"
	"strings"
)

// Media types that may carry ownership. Mirrors the TrackingItem.Type values.
const TypeMusic = "MUSIC"

// Physical formats.
const (
	FormatVinyl = "Vinyl"
	FormatCD    = "CD"
)

// Sentinel errors callers translate into the API envelope.
var (
	// ErrNotSupported means the media type has no ownership vocabulary.
	ErrNotSupported = errors.New("ownership: not supported for this media type")
	// ErrInvalidFormat means a format is not in the type's allowed set.
	ErrInvalidFormat = errors.New("ownership: invalid format")
)

// allowedFormats is the closed vocabulary of ownable formats per media type.
// Adding a media type here is all it takes to give it ownership support.
var allowedFormats = map[string][]string{
	TypeMusic: {FormatVinyl, FormatCD},
}

// AllowedFormats returns the ordered allowed formats for a media type and
// whether the type supports ownership at all.
func AllowedFormats(mediaType string) ([]string, bool) {
	f, ok := allowedFormats[mediaType]
	return f, ok
}

// Validate reports whether every format is allowed for the media type.
// Duplicates are tolerated (Normalize dedupes); an unknown format or an
// unsupported media type is an error.
func Validate(mediaType string, formats []string) error {
	allowed, ok := allowedFormats[mediaType]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotSupported, mediaType)
	}
	for _, f := range formats {
		if !contains(allowed, f) {
			return fmt.Errorf("%w: %q is not one of %v", ErrInvalidFormat, f, allowed)
		}
	}
	return nil
}

// Normalize dedupes formats and orders them by the media type's allowed order,
// then joins them into the stored comma-separated string. Formats are assumed
// already validated. Unknown media types (or empty input) yield "".
func Normalize(mediaType string, formats []string) string {
	allowed, ok := allowedFormats[mediaType]
	if !ok {
		return ""
	}
	seen := make(map[string]bool, len(formats))
	for _, f := range formats {
		seen[f] = true
	}
	ordered := make([]string, 0, len(seen))
	for _, f := range allowed {
		if seen[f] {
			ordered = append(ordered, f)
		}
	}
	return strings.Join(ordered, ",")
}

// Split parses a stored comma-joined ownership string back into a slice,
// preserving the stored (allowed-order) sequence. An empty string yields an
// empty (non-nil) slice.
func Split(stored string) []string {
	if strings.TrimSpace(stored) == "" {
		return []string{}
	}
	parts := strings.Split(stored, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
