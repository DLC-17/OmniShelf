// Package logistics provides physical storage management, container classification,
// binder sleeve tracking, and SVG label generation (Avery 5160 sheets, 4x6 / 2x1 thermal labels)
// for OmniShelf.
package logistics

import (
	"errors"
	"time"
)

// ContainerType classifies a physical storage unit.
type ContainerType string

const (
	ContainerShelf   ContainerType = "SHELF"
	ContainerBinder  ContainerType = "BINDER"
	ContainerBox     ContainerType = "BOX"
	ContainerDrawer  ContainerType = "DRAWER"
	ContainerCabinet ContainerType = "CABINET"
	ContainerTote    ContainerType = "TOTE"
	ContainerOther   ContainerType = "OTHER"
)

// LabelFormat defines printable label sheet or single label dimensions.
type LabelFormat string

const (
	FormatAvery5160 LabelFormat = "avery5160" // 30-up sheet on US Letter (2.625" x 1")
	FormatThermal4x6 LabelFormat = "thermal4x6" // 4" x 6" shipping / storage tote label
	FormatThermal2x1 LabelFormat = "thermal2x1" // 2" x 1" shelf edge / bin / spine label
)

// Sentinel errors.
var (
	ErrInvalidFormat     = errors.New("logistics: invalid label format")
	ErrEmptyLocation     = errors.New("logistics: location cannot be empty")
	ErrInvalidBinderSlot = errors.New("logistics: invalid binder slot")
	ErrInvalidBinderPage = errors.New("logistics: invalid binder page")
	ErrLocationNotFound  = errors.New("logistics: location not found")
)

// StorageContainer represents a physical storage unit and its aggregated contents.
type StorageContainer struct {
	Name          string          `json:"name"`
	Type          ContainerType   `json:"type"`
	Description   string          `json:"description,omitempty"`
	ItemCount     int             `json:"itemCount"`
	ItemBreakdown map[string]int  `json:"itemBreakdown"`
	Capacity      int             `json:"capacity,omitempty"`
	FilterURL     string          `json:"filterUrl"`
	Items         []ContainerItem `json:"items,omitempty"`
}

// ContainerItem is a lightweight representation of an item in a storage container.
type ContainerItem struct {
	ID          uint      `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	ExternalID  string    `json:"externalId"`
	Status      string    `json:"status"`
	SubLocation string    `json:"subLocation,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// BinderSleeveLayout describes the grid arrangement of pocket sleeves in a binder page.
type BinderSleeveLayout struct {
	Name     string `json:"name"`
	Rows     int    `json:"rows"`
	Cols     int    `json:"cols"`
	Capacity int    `json:"capacity"`
}

// Standard sleeve layouts.
var (
	Layout9Pocket = BinderSleeveLayout{Name: "9-Pocket (3x3)", Rows: 3, Cols: 3, Capacity: 9}
	Layout4Pocket = BinderSleeveLayout{Name: "4-Pocket (2x2)", Rows: 2, Cols: 2, Capacity: 4}
	Layout2Pocket = BinderSleeveLayout{Name: "2-Pocket (1x2)", Rows: 1, Cols: 2, Capacity: 2}
	Layout1Pocket = BinderSleeveLayout{Name: "1-Pocket (Single)", Rows: 1, Cols: 1, Capacity: 1}
)

// BinderPosition locates a card within a specific binder page and slot.
type BinderPosition struct {
	BinderName string `json:"binderName"`
	Page       int    `json:"page"`
	Slot       int    `json:"slot"`
	Row        int    `json:"row,omitempty"`
	Col        int    `json:"col,omitempty"`
}

// BinderDetails contains binder-specific summary metrics.
type BinderDetails struct {
	BinderName string `json:"binderName"`
	Page       int    `json:"page,omitempty"`
	Slot       int    `json:"slot,omitempty"`
	PageCount  int    `json:"pageCount,omitempty"`
	SlotsUsed  int    `json:"slotsUsed,omitempty"`
}

// ShelfLabel contains the rendered metadata for generating a single printable label.
type ShelfLabel struct {
	Location      string         `json:"location"`
	Title         string         `json:"title"`
	Subtitle      string         `json:"subtitle,omitempty"`
	ContainerType ContainerType  `json:"containerType"`
	Barcode       string         `json:"barcode"`
	FilterURL     string         `json:"filterUrl"`
	ItemCount     int            `json:"itemCount"`
	ItemTypes     map[string]int `json:"itemTypes,omitempty"`
	ItemPreviews  []string       `json:"itemPreviews,omitempty"`
}

// LabelOptions configures SVG label generation.
type LabelOptions struct {
	Format         LabelFormat `json:"format"`
	BaseURL        string      `json:"baseUrl,omitempty"`
	IncludeBarcode bool        `json:"includeBarcode"`
	IncludeQR      bool        `json:"includeQR"`
	IncludeBorder  bool        `json:"includeBorder"`
	Page           int         `json:"page,omitempty"` // for paginated sheets (1-indexed)
}

// LocationSummary is returned by the locations API endpoint.
type LocationSummary struct {
	Location      string          `json:"location"`
	ContainerType ContainerType   `json:"containerType"`
	ItemCount     int             `json:"itemCount"`
	ItemTypes     map[string]int  `json:"itemTypes"`
	FilterURL     string          `json:"filterUrl"`
	BinderDetails *BinderDetails  `json:"binderDetails,omitempty"`
	Items         []ContainerItem `json:"items,omitempty"`
}

// LocationFilter filters locations returned by the logistics service.
type LocationFilter struct {
	Type          string        // media type (e.g. "BOOK", "CARD")
	ContainerType ContainerType // container type (e.g. ContainerShelf, ContainerBinder)
}
