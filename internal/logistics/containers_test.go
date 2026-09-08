package logistics

import (
	"testing"
)

func TestDetectContainerType(t *testing.T) {
	tests := []struct {
		input    string
		expected ContainerType
	}{
		{"Shelf A", ContainerShelf},
		{"Main Bookcase Top Shelf", ContainerShelf},
		{"Closet Rack 3", ContainerShelf},
		{"Manga Cubby", ContainerShelf},
		{"Binder #1", ContainerBinder},
		{"Pokemon Binder (P1/S2)", ContainerBinder},
		{"Trading Card Album", ContainerBinder},
		{"Deck Box Blue", ContainerBinder},
		{"Toploader Box", ContainerBinder},
		{"Storage Box #4", ContainerBox},
		{"Plastic Tote B", ContainerBox},
		{"Wooden Crate 2", ContainerBox},
		{"Desk Drawer 1", ContainerDrawer},
		{"Nightstand Top Drawer", ContainerDrawer},
		{"Living Room Cabinet", ContainerCabinet},
		{"Bedroom Armoire", ContainerCabinet},
		{"", ContainerOther},
		{"   ", ContainerOther},
		{"Random Room Corner", ContainerShelf}, // default
	}

	for _, tt := range tests {
		got := DetectContainerType(tt.input)
		if got != tt.expected {
			t.Errorf("DetectContainerType(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeLocation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Shelf   A  ", "Shelf A"},
		{"Binder\t#1\n(P2/S3)", "Binder #1 (P2/S3)"},
		{"", ""},
	}

	for _, tt := range tests {
		got := NormalizeLocation(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeLocation(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatFilterURL(t *testing.T) {
	tests := []struct {
		baseURL  string
		location string
		expected string
	}{
		{"", "Shelf A", "/library?location=Shelf+A"},
		{"http://localhost:8080", "Shelf A", "http://localhost:8080/library?location=Shelf+A"},
		{"https://shelf.local/", "Binder 1 (P2/S3)", "https://shelf.local/library?location=Binder+1+%28P2%2FS3%29"},
	}

	for _, tt := range tests {
		got := FormatFilterURL(tt.baseURL, tt.location)
		if got != tt.expected {
			t.Errorf("FormatFilterURL(%q, %q) = %q; want %q", tt.baseURL, tt.location, got, tt.expected)
		}
	}
}

func TestGenerateBarcodeData(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Shelf A", "LOC:Shelf A"},
		{"LOC:Shelf A", "LOC:Shelf A"},
		{"", "LOC:EMPTY"},
		{"   ", "LOC:EMPTY"},
	}

	for _, tt := range tests {
		got := GenerateBarcodeData(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateBarcodeData(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}
