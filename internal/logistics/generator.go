package logistics

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/boombuler/barcode/code128"
	"github.com/skip2/go-qrcode"
)

// BarRect represents a continuous black bar strip in a 1D barcode.
type BarRect struct {
	X     int
	Width int
}

// RenderQRCodeInnerPath generates SVG path commands for QR code modules.
// quietZone defines the module margin around the QR code (typically 1 or 2).
func RenderQRCodeInnerPath(content string, quietZone int) (pathData string, matrixSize int, err error) {
	if quietZone < 0 {
		quietZone = 0
	}
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", 0, fmt.Errorf("generating qr code: %w", err)
	}
	bm := qr.Bitmap()
	size := len(bm)
	totalSize := size + 2*quietZone

	var sb strings.Builder
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if bm[y][x] {
				fmt.Fprintf(&sb, "M%d,%dh1v1h-1z ", x+quietZone, y+quietZone)
			}
		}
	}
	return sb.String(), totalSize, nil
}

// GenerateQRCodeSVG creates a standalone vector SVG string for a QR code.
func GenerateQRCodeSVG(content string, size int) (string, error) {
	path, totalSize, err := RenderQRCodeInnerPath(content, 2)
	if err != nil {
		return "", err
	}
	if size <= 0 {
		size = 200
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" shape-rendering="crispEdges">`+
		`<rect width="%d" height="%d" fill="#ffffff"/>`+
		`<path fill="#000000" d="%s"/>`+
		`</svg>`, totalSize, totalSize, size, size, totalSize, totalSize, path), nil
}

// RenderBarcodeBars calculates the horizontal bar strips for a Code128 barcode.
func RenderBarcodeBars(content string) ([]BarRect, int, error) {
	clean := GenerateBarcodeData(content)
	bc, err := code128.Encode(clean)
	if err != nil {
		// Fallback to simple alphanumeric encoding if special characters fail
		clean = "LOC:" + strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return '_'
		}, content)
		bc, err = code128.Encode(clean)
		if err != nil {
			return nil, 0, fmt.Errorf("encoding barcode: %w", err)
		}
	}
	bounds := bc.Bounds()
	width := bounds.Dx()

	var bars []BarRect
	inBar := false
	barStart := 0

	for x := 0; x < width; x++ {
		r, _, _, a := bc.At(x, 0).RGBA()
		isBlack := a > 0 && r < 0x8000
		if isBlack {
			if !inBar {
				inBar = true
				barStart = x
			}
		} else {
			if inBar {
				bars = append(bars, BarRect{X: barStart, Width: x - barStart})
				inBar = false
			}
		}
	}
	if inBar {
		bars = append(bars, BarRect{X: barStart, Width: width - barStart})
	}
	return bars, width, nil
}

// GenerateBarcodeSVG creates a standalone vector SVG string for a Code128 barcode.
func GenerateBarcodeSVG(content string, width, height int) (string, error) {
	bars, totalWidth, err := RenderBarcodeBars(content)
	if err != nil {
		return "", err
	}
	if width <= 0 {
		width = 240
	}
	if height <= 0 {
		height = 60
	}
	quietZone := 10
	totalViewWidth := totalWidth + 2*quietZone
	totalViewHeight := 50

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" shape-rendering="crispEdges">`,
		totalViewWidth, totalViewHeight+15, width, height)
	fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#ffffff"/>`, totalViewWidth, totalViewHeight+15)
	for _, b := range bars {
		fmt.Fprintf(&sb, `<rect x="%d" y="0" width="%d" height="%d" fill="#000000"/>`, b.X+quietZone, b.Width, totalViewHeight)
	}
	fmt.Fprintf(&sb, `<text x="%d" y="%d" font-family="monospace, sans-serif" font-size="10" text-anchor="middle" fill="#000000">%s</text>`,
		totalViewWidth/2, totalViewHeight+12, html.EscapeString(content))
	sb.WriteString(`</svg>`)
	return sb.String(), nil
}

// GenerateAvery5160Sheet generates a printable 30-label US Letter (8.5" x 11") sheet SVG.
// Dimensions in points (1 in = 72 pt):
// Page: 612 x 792 pt
// Label: 189 x 72 pt (2.625" x 1.0")
// Left Margin: 13.5 pt (0.1875"), Top Margin: 36 pt (0.5")
// Horiz Pitch: 198 pt (2.75"), Vert Pitch: 72 pt (1.0")
func GenerateAvery5160Sheet(labels []ShelfLabel, opts LabelOptions) (string, error) {
	const (
		pageWidth   = 612.0
		pageHeight  = 792.0
		leftMargin  = 13.5
		topMargin   = 36.0
		horizPitch  = 198.0
		vertPitch   = 72.0
		labelWidth  = 189.0
		labelHeight = 72.0
		cols        = 3
		rows        = 10
		labelsPerPage = cols * rows
	)

	// Determine pagination offset
	pageIdx := opts.Page
	if pageIdx <= 0 {
		pageIdx = 1
	}
	startIdx := (pageIdx - 1) * labelsPerPage
	endIdx := startIdx + labelsPerPage
	if startIdx >= len(labels) && len(labels) > 0 {
		startIdx = 0
		endIdx = labelsPerPage
	}

	var pageLabels []ShelfLabel
	if len(labels) > 0 {
		if endIdx > len(labels) {
			endIdx = len(labels)
		}
		if startIdx < len(labels) {
			pageLabels = labels[startIdx:endIdx]
		}
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="8.5in" height="11in" viewBox="0 0 %d %d" style="background:#ffffff;">`,
		int(pageWidth), int(pageHeight))
	sb.WriteString("\n<style>\n")
	sb.WriteString(".label-title { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 10px; font-weight: 700; fill: #0f172a; }\n")
	sb.WriteString(".label-badge { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 6.5px; font-weight: 600; fill: #475569; }\n")
	sb.WriteString(".label-sub { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 7px; fill: #64748b; }\n")
	sb.WriteString(".label-hint { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 6px; fill: #94a3b8; }\n")
	sb.WriteString("</style>\n")

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			idx := r*cols + c
			cellX := leftMargin + float64(c)*horizPitch
			cellY := topMargin + float64(r)*vertPitch

			// Label container outline / cut guideline
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="3" fill="#ffffff" stroke="#e2e8f0" stroke-width="0.5" stroke-dasharray="2,2"/>`+"\n",
				cellX, cellY, labelWidth, labelHeight)

			if idx >= len(pageLabels) {
				continue
			}

			lbl := pageLabels[idx]
			filterURL := lbl.FilterURL
			if filterURL == "" {
				filterURL = FormatFilterURL(opts.BaseURL, lbl.Location)
			}

			// Left Content Box
			textX := cellX + 7.0
			curY := cellY + 12.0

			// Container Type Badge
			containerType := string(lbl.ContainerType)
			if containerType == "" {
				containerType = string(DetectContainerType(lbl.Location))
			}
			itemCountText := ""
			if lbl.ItemCount > 0 {
				itemCountText = fmt.Sprintf(" • %d items", lbl.ItemCount)
			}
			badgeText := fmt.Sprintf("[%s%s]", containerType, itemCountText)
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="label-badge">%s</text>`+"\n",
				textX, curY, html.EscapeString(badgeText))

			// Location Name
			curY += 13.0
			locName := lbl.Location
			if lbl.Title != "" && lbl.Title != locName {
				locName = lbl.Title
			}
			// Truncate location name if too long
			if len(locName) > 20 {
				locName = locName[:18] + "..."
			}
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="label-title">%s</text>`+"\n",
				textX, curY, html.EscapeString(locName))

			// Subtitle / Preview text
			curY += 11.0
			sub := lbl.Subtitle
			if sub == "" && len(lbl.ItemPreviews) > 0 {
				sub = strings.Join(lbl.ItemPreviews, ", ")
			}
			if len(sub) > 24 {
				sub = sub[:22] + "..."
			}
			if sub != "" {
				fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="label-sub">%s</text>`+"\n",
					textX, curY, html.EscapeString(sub))
			}

			// Barcode / hint line
			curY += 10.0
			barcodeText := lbl.Barcode
			if barcodeText == "" {
				barcodeText = GenerateBarcodeData(lbl.Location)
			}
			if len(barcodeText) > 22 {
				barcodeText = barcodeText[:20] + ".."
			}
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="label-hint">%s</text>`+"\n",
				textX, curY, html.EscapeString(barcodeText))

			// QR Code on right side
			qrSize := 52.0
			qrX := cellX + labelWidth - qrSize - 6.0
			qrY := cellY + (labelHeight-qrSize)/2.0

			qrPath, matrixSize, qrErr := RenderQRCodeInnerPath(filterURL, 1)
			if qrErr == nil && matrixSize > 0 {
				fmt.Fprintf(&sb, `<svg x="%.1f" y="%.1f" width="%.1f" height="%.1f" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+"\n",
					qrX, qrY, qrSize, qrSize, matrixSize, matrixSize)
				fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#ffffff"/>`+"\n", matrixSize, matrixSize)
				fmt.Fprintf(&sb, `<path fill="#000000" d="%s"/>`+"\n", qrPath)
				sb.WriteString("</svg>\n")
			}
		}
	}

	sb.WriteString("</svg>")
	return sb.String(), nil
}

// GenerateThermal4x6 creates a high-density 4" x 6" printable storage container label SVG.
// Dimensions in points (1 in = 72 pt): 288 x 432 pt.
func GenerateThermal4x6(label ShelfLabel, opts LabelOptions) (string, error) {
	const (
		width  = 288.0
		height = 432.0
	)

	filterURL := label.FilterURL
	if filterURL == "" {
		filterURL = FormatFilterURL(opts.BaseURL, label.Location)
	}
	containerType := string(label.ContainerType)
	if containerType == "" {
		containerType = string(DetectContainerType(label.Location))
	}
	barcodeData := label.Barcode
	if barcodeData == "" {
		barcodeData = GenerateBarcodeData(label.Location)
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="4in" height="6in" viewBox="0 0 %d %d" style="background:#ffffff;">`,
		int(width), int(height))
	sb.WriteString("\n<style>\n")
	sb.WriteString(".h-brand { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 8.5px; font-weight: 700; letter-spacing: 1.5px; fill: #64748b; }\n")
	sb.WriteString(".h-badge { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 9px; font-weight: 700; fill: #0f172a; }\n")
	sb.WriteString(".h-loc { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 22px; font-weight: 800; fill: #0f172a; }\n")
	sb.WriteString(".h-sub { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 10px; fill: #475569; }\n")
	sb.WriteString(".sec-title { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 9.5px; font-weight: 700; letter-spacing: 1px; fill: #334155; }\n")
	sb.WriteString(".item-line { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 9px; fill: #1e293b; }\n")
	sb.WriteString(".stat-pill { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 8.5px; font-weight: 600; fill: #0f172a; }\n")
	sb.WriteString(".footer-text { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 7.5px; fill: #94a3b8; }\n")
	sb.WriteString("</style>\n")

	// Outer label border
	sb.WriteString(`<rect x="8" y="8" width="272" height="416" rx="6" fill="#ffffff" stroke="#0f172a" stroke-width="2"/>` + "\n")

	// Header
	sb.WriteString(`<text x="20" y="28" class="h-brand">OMNISHELF STORAGE</text>` + "\n")
	badgeText := fmt.Sprintf("[%s]", containerType)
	fmt.Fprintf(&sb, `<text x="268" y="28" text-anchor="end" class="h-badge">%s</text>`+"\n", html.EscapeString(badgeText))

	// Location Title
	title := label.Location
	if label.Title != "" {
		title = label.Title
	}
	if len(title) > 22 {
		title = title[:20] + "..."
	}
	fmt.Fprintf(&sb, `<text x="20" y="56" class="h-loc">%s</text>`+"\n", html.EscapeString(title))

	// Subtitle / Item Count
	sub := label.Subtitle
	if sub == "" {
		if label.ItemCount > 0 {
			sub = fmt.Sprintf("Storage unit containing %d cataloged items", label.ItemCount)
		} else {
			sub = "Empty storage container"
		}
	}
	if len(sub) > 42 {
		sub = sub[:39] + "..."
	}
	fmt.Fprintf(&sb, `<text x="20" y="74" class="h-sub">%s</text>`+"\n", html.EscapeString(sub))

	// Divider
	sb.WriteString(`<line x1="20" y1="84" x2="268" y2="84" stroke="#cbd5e1" stroke-width="1"/>` + "\n")

	// QR Code (Left) & Barcode (Right)
	qrSize := 104.0
	qrX := 22.0
	qrY := 94.0

	qrPath, matrixSize, qrErr := RenderQRCodeInnerPath(filterURL, 2)
	if qrErr == nil && matrixSize > 0 {
		fmt.Fprintf(&sb, `<svg x="%.1f" y="%.1f" width="%.1f" height="%.1f" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+"\n",
			qrX, qrY, qrSize, qrSize, matrixSize, matrixSize)
		fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#ffffff"/>`+"\n", matrixSize, matrixSize)
		fmt.Fprintf(&sb, `<path fill="#000000" d="%s"/>`+"\n", qrPath)
		sb.WriteString("</svg>\n")
		sb.WriteString(`<text x="74" y="208" text-anchor="middle" class="h-brand">SCAN TO FILTER</text>` + "\n")
	}

	// Barcode (Right)
	bars, bcWidth, bcErr := RenderBarcodeBars(barcodeData)
	if bcErr == nil && bcWidth > 0 {
		bcX := 140.0
		bcY := 108.0
		bcBoxW := 128.0
		bcBoxH := 58.0
		scale := bcBoxW / float64(bcWidth)

		fmt.Fprintf(&sb, `<g transform="translate(%.1f, %.1f)">`+"\n", bcX, bcY)
		for _, b := range bars {
			fmt.Fprintf(&sb, `<rect x="%.2f" y="0" width="%.2f" height="%.1f" fill="#000000"/>`+"\n",
				float64(b.X)*scale, float64(b.Width)*scale, bcBoxH)
		}
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" font-family="monospace" font-size="8" fill="#0f172a">%s</text>`+"\n",
			bcBoxW/2.0, bcBoxH+12.0, html.EscapeString(barcodeData))
		sb.WriteString("</g>\n")
	}

	// Divider
	sb.WriteString(`<line x1="20" y1="220" x2="268" y2="220" stroke="#cbd5e1" stroke-width="1"/>` + "\n")

	// Media Breakdown Pills
	sb.WriteString(`<text x="20" y="238" class="sec-title">CATEGORY BREAKDOWN</text>` + "\n")
	pillX := 20.0
	pillY := 246.0
	if len(label.ItemTypes) > 0 {
		types := make([]string, 0, len(label.ItemTypes))
		for mType := range label.ItemTypes {
			types = append(types, mType)
		}
		strings.Join(types, "") // keep strings import
		// sort keys alphabetically
		for i := 0; i < len(types); i++ {
			for j := i + 1; j < len(types); j++ {
				if types[i] > types[j] {
					types[i], types[j] = types[j], types[i]
				}
			}
		}
		for _, mType := range types {
			count := label.ItemTypes[mType]
			if count <= 0 {
				continue
			}
			pillText := fmt.Sprintf("%s: %d", mType, count)
			pillWidth := float64(len(pillText))*5.5 + 14.0
			if pillX+pillWidth > 268.0 {
				break
			}
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="16" rx="4" fill="#f1f5f9" stroke="#cbd5e1" stroke-width="0.5"/>`+"\n",
				pillX, pillY, pillWidth)
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="stat-pill">%s</text>`+"\n",
				pillX+7.0, pillY+11.5, html.EscapeString(pillText))
			pillX += pillWidth + 6.0
		}
	} else {
		fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="80" height="16" rx="4" fill="#f1f5f9" stroke="#cbd5e1" stroke-width="0.5"/>`+"\n",
			pillX, pillY)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="stat-pill">Total: %d</text>`+"\n",
			pillX+7.0, pillY+11.5, label.ItemCount)
	}

	// Sample Items List
	sb.WriteString(`<text x="20" y="282" class="sec-title">SAMPLE CONTENTS</text>` + "\n")
	itemY := 298.0
	previews := label.ItemPreviews
	if len(previews) == 0 && label.ItemCount > 0 {
		previews = []string{"(Items cataloged on shelf)"}
	}
	for i, prev := range previews {
		if i >= 5 {
			break
		}
		if len(prev) > 36 {
			prev = prev[:33] + "..."
		}
		fmt.Fprintf(&sb, `<text x="20" y="%.1f" class="item-line">• %s</text>`+"\n",
			itemY, html.EscapeString(prev))
		itemY += 14.0
	}

	// Footer
	now := time.Now().Format("2006-01-02")
	fmt.Fprintf(&sb, `<text x="20" y="412" class="footer-text">OMNISHELF LOGISTICS • %s</text>`+"\n", now)
	fmt.Fprintf(&sb, `<text x="268" y="412" text-anchor="end" class="footer-text">%s</text>`+"\n", html.EscapeString(filterURL))

	sb.WriteString("</svg>")
	return sb.String(), nil
}

// GenerateThermal2x1 creates a compact 2" x 1" shelf edge / bin / spine label SVG.
// Dimensions in points (1 in = 72 pt): 144 x 72 pt.
func GenerateThermal2x1(label ShelfLabel, opts LabelOptions) (string, error) {
	const (
		width  = 144.0
		height = 72.0
	)

	filterURL := label.FilterURL
	if filterURL == "" {
		filterURL = FormatFilterURL(opts.BaseURL, label.Location)
	}
	containerType := string(label.ContainerType)
	if containerType == "" {
		containerType = string(DetectContainerType(label.Location))
	}
	barcodeData := label.Barcode
	if barcodeData == "" {
		barcodeData = GenerateBarcodeData(label.Location)
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="2in" height="1in" viewBox="0 0 %d %d" style="background:#ffffff;">`,
		int(width), int(height))
	sb.WriteString("\n<style>\n")
	sb.WriteString(".t2-loc { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 11px; font-weight: 800; fill: #0f172a; }\n")
	sb.WriteString(".t2-badge { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 6.5px; font-weight: 700; fill: #475569; }\n")
	sb.WriteString(".t2-count { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; font-size: 7.5px; fill: #334155; }\n")
	sb.WriteString(".t2-code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 6px; fill: #94a3b8; }\n")
	sb.WriteString("</style>\n")

	// Outer border
	sb.WriteString(`<rect x="2" y="2" width="140" height="68" rx="4" fill="#ffffff" stroke="#0f172a" stroke-width="1.5"/>` + "\n")

	// QR Code on Left
	qrSize := 56.0
	qrX := 6.0
	qrY := 8.0
	qrPath, matrixSize, qrErr := RenderQRCodeInnerPath(filterURL, 1)
	if qrErr == nil && matrixSize > 0 {
		fmt.Fprintf(&sb, `<svg x="%.1f" y="%.1f" width="%.1f" height="%.1f" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+"\n",
			qrX, qrY, qrSize, qrSize, matrixSize, matrixSize)
		fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#ffffff"/>`+"\n", matrixSize, matrixSize)
		fmt.Fprintf(&sb, `<path fill="#000000" d="%s"/>`+"\n", qrPath)
		sb.WriteString("</svg>\n")
	}

	// Right side Content Box
	rightX := 68.0
	curY := 18.0

	// Badge
	badgeText := fmt.Sprintf("[%s]", containerType)
	fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="t2-badge">%s</text>`+"\n",
		rightX, curY, html.EscapeString(badgeText))

	// Location Title
	curY += 14.0
	locName := label.Location
	if label.Title != "" {
		locName = label.Title
	}
	if len(locName) > 18 {
		locName = locName[:16] + ".."
	}
	fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="t2-loc">%s</text>`+"\n",
		rightX, curY, html.EscapeString(locName))

	// Items Count
	curY += 12.0
	countText := "0 items"
	if label.ItemCount > 0 {
		countText = fmt.Sprintf("%d items", label.ItemCount)
	}
	fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="t2-count">%s</text>`+"\n",
		rightX, curY, html.EscapeString(countText))

	// Barcode text
	curY += 11.0
	if len(barcodeData) > 14 {
		barcodeData = barcodeData[:12] + ".."
	}
	fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" class="t2-code">%s</text>`+"\n",
		rightX, curY, html.EscapeString(barcodeData))

	sb.WriteString("</svg>")
	return sb.String(), nil
}

// GenerateLabels dispatches SVG generation to the requested format handler.
func GenerateLabels(format LabelFormat, labels []ShelfLabel, opts LabelOptions) (string, error) {
	opts.Format = format
	switch format {
	case FormatAvery5160, "5160", "sheet", "avery_5160":
		return GenerateAvery5160Sheet(labels, opts)
	case FormatThermal4x6, "4x6", "thermal_4x6":
		var first ShelfLabel
		if len(labels) > 0 {
			first = labels[0]
		} else {
			first = ShelfLabel{Location: "OmniShelf", ContainerType: ContainerShelf}
		}
		return GenerateThermal4x6(first, opts)
	case FormatThermal2x1, "2x1", "thermal_2x1":
		var first ShelfLabel
		if len(labels) > 0 {
			first = labels[0]
		} else {
			first = ShelfLabel{Location: "OmniShelf", ContainerType: ContainerShelf}
		}
		return GenerateThermal2x1(first, opts)
	default:
		return "", fmt.Errorf("%w: %q (supported: avery5160, thermal4x6, thermal2x1)", ErrInvalidFormat, format)
	}
}
