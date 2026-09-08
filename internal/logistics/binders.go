package logistics

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Matches:
	// "Binder 1 - Page 2, Slot 3"
	// "Binder 1 (P2/S3)"
	// "Binder #1 (Page 2, Slot 4)"
	// "Binder A [P3-S9]"
	// "Binder 2 - P4 S1"
	binderPageSlotRegex = regexp.MustCompile(`(?i)^(.*?)\s*[-–—:]?\s*(?:\(|\[)?\s*p(?:age)?\.?\s*(\d+)\s*[,/|\-\s]\s*s(?:lot|leeve|pot)?\.?\s*(\d+)\s*(?:\)|\])?$`)
	// Matches: "Binder 1 - Page 2", "Binder 1 (P2)"
	binderPageRegex = regexp.MustCompile(`(?i)^(.*?)\s*[-–—:]?\s*(?:\(|\[)?\s*p(?:age)?\.?\s*(\d+)\s*(?:\)|\])?$`)
	// Matches: "Binder 1 - Slot 3", "Binder 1 (S3)"
	binderSlotRegex = regexp.MustCompile(`(?i)^(.*?)\s*[-–—:]?\s*(?:\(|\[)?\s*s(?:lot|leeve|pot)?\.?\s*(\d+)\s*(?:\)|\])?$`)
)

// ParseBinderLocation attempts to extract binder name, page number, and slot number.
func ParseBinderLocation(location string) (binderName string, pos *BinderPosition, ok bool) {
	loc := strings.TrimSpace(location)
	if loc == "" {
		return "", nil, false
	}

	if m := binderPageSlotRegex.FindStringSubmatch(loc); len(m) == 4 {
		bName := strings.TrimSpace(m[1])
		page, _ := strconv.Atoi(m[2])
		slot, _ := strconv.Atoi(m[3])
		row, col, _ := SlotToGrid(slot, Layout9Pocket)
		return bName, &BinderPosition{
			BinderName: bName,
			Page:       page,
			Slot:       slot,
			Row:        row,
			Col:        col,
		}, true
	}

	if m := binderPageRegex.FindStringSubmatch(loc); len(m) == 3 {
		bName := strings.TrimSpace(m[1])
		page, _ := strconv.Atoi(m[2])
		return bName, &BinderPosition{
			BinderName: bName,
			Page:       page,
			Slot:       0,
		}, true
	}

	if m := binderSlotRegex.FindStringSubmatch(loc); len(m) == 3 {
		bName := strings.TrimSpace(m[1])
		slot, _ := strconv.Atoi(m[2])
		row, col, _ := SlotToGrid(slot, Layout9Pocket)
		return bName, &BinderPosition{
			BinderName: bName,
			Page:       0,
			Slot:       slot,
			Row:        row,
			Col:        col,
		}, true
	}

	if DetectContainerType(loc) == ContainerBinder {
		return loc, &BinderPosition{
			BinderName: loc,
		}, true
	}

	return "", nil, false
}

// FormatBinderLocation creates a standardized location string.
// E.g. FormatBinderLocation("Binder 1", 2, 4) -> "Binder 1 (P2/S4)"
func FormatBinderLocation(binderName string, page, slot int) string {
	binderName = strings.TrimSpace(binderName)
	if page > 0 && slot > 0 {
		return fmt.Sprintf("%s (P%d/S%d)", binderName, page, slot)
	}
	if page > 0 {
		return fmt.Sprintf("%s (Page %d)", binderName, page)
	}
	if slot > 0 {
		return fmt.Sprintf("%s (Slot %d)", binderName, slot)
	}
	return binderName
}

// SlotToGrid converts a 1-indexed slot number to 1-indexed (row, col) coordinates.
func SlotToGrid(slot int, layout BinderSleeveLayout) (row, col int, err error) {
	if layout.Cols <= 0 || layout.Rows <= 0 {
		layout = Layout9Pocket
	}
	if slot <= 0 {
		return 0, 0, fmt.Errorf("%w: slot must be >= 1 (got %d)", ErrInvalidBinderSlot, slot)
	}
	if layout.Capacity > 0 && slot > layout.Capacity {
		return 0, 0, fmt.Errorf("%w: slot %d exceeds layout capacity %d", ErrInvalidBinderSlot, slot, layout.Capacity)
	}

	index := slot - 1
	row = (index / layout.Cols) + 1
	col = (index % layout.Cols) + 1
	return row, col, nil
}

// GridToSlot converts 1-indexed (row, col) coordinates to a 1-indexed slot number.
func GridToSlot(row, col int, layout BinderSleeveLayout) (slot int, err error) {
	if layout.Cols <= 0 || layout.Rows <= 0 {
		layout = Layout9Pocket
	}
	if row <= 0 || row > layout.Rows {
		return 0, fmt.Errorf("%w: row %d outside 1..%d", ErrInvalidBinderSlot, row, layout.Rows)
	}
	if col <= 0 || col > layout.Cols {
		return 0, fmt.Errorf("%w: col %d outside 1..%d", ErrInvalidBinderSlot, col, layout.Cols)
	}

	slot = (row-1)*layout.Cols + col
	return slot, nil
}

// CalculateBinderCapacity computes the maximum number of cards a binder can hold.
func CalculateBinderCapacity(pages int, layout BinderSleeveLayout) int {
	if pages <= 0 {
		return 0
	}
	cap := layout.Capacity
	if cap <= 0 {
		cap = 9
	}
	return pages * cap
}
