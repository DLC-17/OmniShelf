package logistics

import (
	"errors"
	"testing"
)

func TestParseBinderLocation(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantPage    int
		wantSlot    int
		wantRow     int
		wantCol     int
		wantSuccess bool
	}{
		{
			input:       "Binder 1 - Page 2, Slot 3",
			wantName:    "Binder 1",
			wantPage:    2,
			wantSlot:    3,
			wantRow:     1,
			wantCol:     3,
			wantSuccess: true,
		},
		{
			input:       "Pokemon Binder (P2/S4)",
			wantName:    "Pokemon Binder",
			wantPage:    2,
			wantSlot:    4,
			wantRow:     2,
			wantCol:     1,
			wantSuccess: true,
		},
		{
			input:       "YuGiOh Binder [P3-S9]",
			wantName:    "YuGiOh Binder",
			wantPage:    3,
			wantSlot:    9,
			wantRow:     3,
			wantCol:     3,
			wantSuccess: true,
		},
		{
			input:       "Card Binder - Page 5",
			wantName:    "Card Binder",
			wantPage:    5,
			wantSlot:    0,
			wantSuccess: true,
		},
		{
			input:       "Binder 2 - Slot 7",
			wantName:    "Binder 2",
			wantPage:    0,
			wantSlot:    7,
			wantRow:     3,
			wantCol:     1,
			wantSuccess: true,
		},
		{
			input:       "Main Binder",
			wantName:    "Main Binder",
			wantPage:    0,
			wantSlot:    0,
			wantSuccess: true,
		},
		{
			input:       "Shelf A",
			wantSuccess: false,
		},
		{
			input:       "",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		name, pos, ok := ParseBinderLocation(tt.input)
		if ok != tt.wantSuccess {
			t.Errorf("ParseBinderLocation(%q) ok = %v; want %v", tt.input, ok, tt.wantSuccess)
			continue
		}
		if !ok {
			continue
		}
		if name != tt.wantName {
			t.Errorf("ParseBinderLocation(%q) name = %q; want %q", tt.input, name, tt.wantName)
		}
		if pos != nil {
			if pos.Page != tt.wantPage {
				t.Errorf("ParseBinderLocation(%q) pos.Page = %d; want %d", tt.input, pos.Page, tt.wantPage)
			}
			if pos.Slot != tt.wantSlot {
				t.Errorf("ParseBinderLocation(%q) pos.Slot = %d; want %d", tt.input, pos.Slot, tt.wantSlot)
			}
			if tt.wantRow > 0 && pos.Row != tt.wantRow {
				t.Errorf("ParseBinderLocation(%q) pos.Row = %d; want %d", tt.input, pos.Row, tt.wantRow)
			}
			if tt.wantCol > 0 && pos.Col != tt.wantCol {
				t.Errorf("ParseBinderLocation(%q) pos.Col = %d; want %d", tt.input, pos.Col, tt.wantCol)
			}
		}
	}
}

func TestFormatBinderLocation(t *testing.T) {
	if got := FormatBinderLocation("Binder 1", 2, 5); got != "Binder 1 (P2/S5)" {
		t.Errorf("FormatBinderLocation(..., 2, 5) = %q; want %q", got, "Binder 1 (P2/S5)")
	}
	if got := FormatBinderLocation("Binder 1", 3, 0); got != "Binder 1 (Page 3)" {
		t.Errorf("FormatBinderLocation(..., 3, 0) = %q; want %q", got, "Binder 1 (Page 3)")
	}
	if got := FormatBinderLocation("Binder 1", 0, 4); got != "Binder 1 (Slot 4)" {
		t.Errorf("FormatBinderLocation(..., 0, 4) = %q; want %q", got, "Binder 1 (Slot 4)")
	}
	if got := FormatBinderLocation("Binder 1", 0, 0); got != "Binder 1" {
		t.Errorf("FormatBinderLocation(..., 0, 0) = %q; want %q", got, "Binder 1")
	}
}

func TestSlotToGridAndGridToSlot(t *testing.T) {
	layout := Layout9Pocket // 3x3, cap=9

	for slot := 1; slot <= 9; slot++ {
		row, col, err := SlotToGrid(slot, layout)
		if err != nil {
			t.Fatalf("SlotToGrid(%d) unexpected error: %v", slot, err)
		}
		backSlot, err := GridToSlot(row, col, layout)
		if err != nil {
			t.Fatalf("GridToSlot(%d, %d) unexpected error: %v", row, col, err)
		}
		if backSlot != slot {
			t.Errorf("roundtrip failed: slot %d -> (r=%d, c=%d) -> slot %d", slot, row, col, backSlot)
		}
	}

	// Boundary checks
	if _, _, err := SlotToGrid(0, layout); !errors.Is(err, ErrInvalidBinderSlot) {
		t.Errorf("SlotToGrid(0) expected ErrInvalidBinderSlot; got %v", err)
	}
	if _, _, err := SlotToGrid(10, layout); !errors.Is(err, ErrInvalidBinderSlot) {
		t.Errorf("SlotToGrid(10) expected ErrInvalidBinderSlot; got %v", err)
	}
	if _, err := GridToSlot(0, 1, layout); !errors.Is(err, ErrInvalidBinderSlot) {
		t.Errorf("GridToSlot(0, 1) expected ErrInvalidBinderSlot; got %v", err)
	}
	if _, err := GridToSlot(4, 1, layout); !errors.Is(err, ErrInvalidBinderSlot) {
		t.Errorf("GridToSlot(4, 1) expected ErrInvalidBinderSlot; got %v", err)
	}
}

func TestCalculateBinderCapacity(t *testing.T) {
	if got := CalculateBinderCapacity(20, Layout9Pocket); got != 180 {
		t.Errorf("CalculateBinderCapacity(20, 9-pocket) = %d; want 180", got)
	}
	if got := CalculateBinderCapacity(10, Layout4Pocket); got != 40 {
		t.Errorf("CalculateBinderCapacity(10, 4-pocket) = %d; want 40", got)
	}
	if got := CalculateBinderCapacity(0, Layout9Pocket); got != 0 {
		t.Errorf("CalculateBinderCapacity(0, 9-pocket) = %d; want 0", got)
	}
}
