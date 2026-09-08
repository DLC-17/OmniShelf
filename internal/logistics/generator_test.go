package logistics

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateQRCodeSVG(t *testing.T) {
	svg, err := GenerateQRCodeSVG("https://omnishelf.local/library?location=Shelf+A", 150)
	if err != nil {
		t.Fatalf("GenerateQRCodeSVG failed: %v", err)
	}
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Errorf("SVG missing root tags: %s", svg)
	}
	if !strings.Contains(svg, `width="150"`) || !strings.Contains(svg, `height="150"`) {
		t.Errorf("SVG missing requested width/height: %s", svg)
	}
	if !strings.Contains(svg, "<path") {
		t.Errorf("SVG missing QR module path: %s", svg)
	}
}

func TestGenerateBarcodeSVG(t *testing.T) {
	svg, err := GenerateBarcodeSVG("LOC:SHELF-A", 200, 50)
	if err != nil {
		t.Fatalf("GenerateBarcodeSVG failed: %v", err)
	}
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Errorf("SVG missing root tags: %s", svg)
	}
	if !strings.Contains(svg, `width="200"`) || !strings.Contains(svg, `height="50"`) {
		t.Errorf("SVG missing requested width/height: %s", svg)
	}
	if !strings.Contains(svg, "LOC:SHELF-A") {
		t.Errorf("SVG missing human-readable text: %s", svg)
	}
}

func TestGenerateAvery5160Sheet(t *testing.T) {
	labels := []ShelfLabel{
		{
			Location:      "Shelf A",
			ContainerType: ContainerShelf,
			ItemCount:     15,
			ItemPreviews:  []string{"Dune", "Neuromancer"},
		},
		{
			Location:      "Binder #1 (P1/S1)",
			ContainerType: ContainerBinder,
			ItemCount:     1,
			ItemPreviews:  []string{"Charizard Base Set"},
		},
		{
			Location:      "Box 3",
			ContainerType: ContainerBox,
			ItemCount:     8,
			ItemPreviews:  []string{"PlayStation Games"},
		},
	}

	opts := LabelOptions{
		Format:  FormatAvery5160,
		BaseURL: "http://omnishelf.local:8080",
	}

	svg, err := GenerateAvery5160Sheet(labels, opts)
	if err != nil {
		t.Fatalf("GenerateAvery5160Sheet failed: %v", err)
	}

	if !strings.Contains(svg, `width="8.5in"`) || !strings.Contains(svg, `height="11in"`) {
		t.Errorf("Avery 5160 SVG missing 8.5in x 11in dimensions: %s", svg)
	}
	if !strings.Contains(svg, `viewBox="0 0 612 792"`) {
		t.Errorf("Avery 5160 SVG missing viewBox: %s", svg)
	}
	if !strings.Contains(svg, "Shelf A") {
		t.Errorf("Avery 5160 SVG missing label 1: %s", svg)
	}
	if !strings.Contains(svg, "Binder #1 (P1/S1)") {
		t.Errorf("Avery 5160 SVG missing label 2: %s", svg)
	}
}

func TestGenerateThermal4x6(t *testing.T) {
	label := ShelfLabel{
		Location:      "Shelf A",
		Title:         "Living Room Main Shelf",
		ContainerType: ContainerShelf,
		ItemCount:     24,
		ItemTypes: map[string]int{
			"BOOK": 18,
			"GAME": 6,
		},
		ItemPreviews: []string{"Dune", "The Hobbit", "Elden Ring"},
	}

	opts := LabelOptions{
		Format:  FormatThermal4x6,
		BaseURL: "https://omnishelf.local",
	}

	svg, err := GenerateThermal4x6(label, opts)
	if err != nil {
		t.Fatalf("GenerateThermal4x6 failed: %v", err)
	}

	if !strings.Contains(svg, `width="4in"`) || !strings.Contains(svg, `height="6in"`) {
		t.Errorf("Thermal 4x6 SVG missing 4in x 6in dimensions: %s", svg)
	}
	if !strings.Contains(svg, `viewBox="0 0 288 432"`) {
		t.Errorf("Thermal 4x6 SVG missing viewBox: %s", svg)
	}
	if !strings.Contains(svg, "Living Room Main Shelf") {
		t.Errorf("Thermal 4x6 SVG missing title: %s", svg)
	}
	if !strings.Contains(svg, "BOOK: 18") {
		t.Errorf("Thermal 4x6 SVG missing category breakdown: %s", svg)
	}
	if !strings.Contains(svg, "• Dune") {
		t.Errorf("Thermal 4x6 SVG missing preview item: %s", svg)
	}
}

func TestGenerateThermal2x1(t *testing.T) {
	label := ShelfLabel{
		Location:      "Shelf A",
		ContainerType: ContainerShelf,
		ItemCount:     12,
	}

	opts := LabelOptions{
		Format:  FormatThermal2x1,
		BaseURL: "http://localhost:8080",
	}

	svg, err := GenerateThermal2x1(label, opts)
	if err != nil {
		t.Fatalf("GenerateThermal2x1 failed: %v", err)
	}

	if !strings.Contains(svg, `width="2in"`) || !strings.Contains(svg, `height="1in"`) {
		t.Errorf("Thermal 2x1 SVG missing 2in x 1in dimensions: %s", svg)
	}
	if !strings.Contains(svg, `viewBox="0 0 144 72"`) {
		t.Errorf("Thermal 2x1 SVG missing viewBox: %s", svg)
	}
	if !strings.Contains(svg, "Shelf A") {
		t.Errorf("Thermal 2x1 SVG missing location: %s", svg)
	}
	if !strings.Contains(svg, "12 items") {
		t.Errorf("Thermal 2x1 SVG missing item count: %s", svg)
	}
}

func TestGenerateLabelsDispatcher(t *testing.T) {
	labels := []ShelfLabel{
		{Location: "Shelf A", ContainerType: ContainerShelf, ItemCount: 5},
	}

	for _, format := range []LabelFormat{FormatAvery5160, FormatThermal4x6, FormatThermal2x1} {
		svg, err := GenerateLabels(format, labels, LabelOptions{})
		if err != nil {
			t.Errorf("GenerateLabels(%s) unexpected error: %v", format, err)
		}
		if len(svg) == 0 {
			t.Errorf("GenerateLabels(%s) returned empty SVG", format)
		}
	}

	_, err := GenerateLabels("unknown_format", labels, LabelOptions{})
	if !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("GenerateLabels(unknown) expected ErrInvalidFormat; got %v", err)
	}
}
