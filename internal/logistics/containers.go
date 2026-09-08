package logistics

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	shelfRegex   = regexp.MustCompile(`(?i)\b(shelf|shelves|bookcase|bookshelf|rack|cubby|closet|ledge|mantel|display)\b`)
	binderRegex  = regexp.MustCompile(`(?i)\b(binder|sleeve|album|portfolio|deck\s*box|toploader|card\s*box)\b`)
	boxRegex     = regexp.MustCompile(`(?i)\b(box|tote|crate|bin|container|tub|storage\s*box|shoebox)\b`)
	drawerRegex  = regexp.MustCompile(`(?i)\b(drawer|nightstand|dresser|desk\s*drawer)\b`)
	cabinetRegex = regexp.MustCompile(`(?i)\b(cabinet|cupboard|wardrobe|armoire|credenza|sideboard)\b`)
)

// DetectContainerType analyzes a location name and classifies its container type.
func DetectContainerType(location string) ContainerType {
	loc := strings.TrimSpace(location)
	if loc == "" {
		return ContainerOther
	}

	switch {
	case binderRegex.MatchString(loc):
		return ContainerBinder
	case shelfRegex.MatchString(loc):
		return ContainerShelf
	case drawerRegex.MatchString(loc):
		return ContainerDrawer
	case cabinetRegex.MatchString(loc):
		return ContainerCabinet
	case boxRegex.MatchString(loc):
		return ContainerBox
	default:
		return ContainerShelf // default physical location assumption
	}
}

// NormalizeLocation cleans and collapses whitespace in a location string.
func NormalizeLocation(loc string) string {
	return strings.Join(strings.Fields(loc), " ")
}

// FormatFilterURL builds the library filter URL for a given location.
func FormatFilterURL(baseURL, location string) string {
	encoded := url.QueryEscape(location)
	path := "/library?location=" + encoded
	if baseURL == "" {
		return path
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + path
}

// GenerateBarcodeData returns a standardized Code128 payload for a location.
func GenerateBarcodeData(location string) string {
	clean := strings.TrimSpace(location)
	if clean == "" {
		return "LOC:EMPTY"
	}
	// Code128 supports ASCII characters 32 to 126.
	var b strings.Builder
	for _, r := range clean {
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := b.String()
	if !strings.HasPrefix(strings.ToUpper(res), "LOC:") {
		return "LOC:" + res
	}
	return res
}
