package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// Icons used across the UI, parsed once. They are simple stroke icons in
// the style of Feather, drawn in the color of the text.

const iconTemplate = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">%s</svg>`

var (
	iconTerminal = ui.MustParseSVG([]byte(withIconBody(
		`<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>`)))
	iconPlus = ui.MustParseSVG([]byte(withIconBody(
		`<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>`)))
	iconEdit = ui.MustParseSVG([]byte(withIconBody(
		`<path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>`)))
	iconTrash = ui.MustParseSVG([]byte(withIconBody(
		`<polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/>`)))
	iconClose = ui.MustParseSVG([]byte(withIconBody(
		`<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>`)))
	iconFile = ui.MustParseSVG([]byte(withIconBody(
		`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/>`)))
	iconDocument = ui.MustParseSVG([]byte(withIconBody(
		`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/>`)))
	iconComputer = ui.MustParseSVG([]byte(withIconBody(
		`<rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/>`)))
	iconInfo = ui.MustParseSVG([]byte(withIconBody(
		`<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>`)))
	iconKey = ui.MustParseSVG([]byte(withIconBody(
		`<path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>`)))
	iconBox = ui.MustParseSVG([]byte(withIconBody(
		`<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>`)))
	iconCloud = ui.MustParseSVG([]byte(withIconBody(
		`<path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z"/>`)))
	iconChevronUp = ui.MustParseSVG([]byte(withIconBody(
		`<polyline points="18 15 12 9 6 15"/>`)))
	iconChevronDown = ui.MustParseSVG([]byte(withIconBody(
		`<polyline points="6 9 12 15 18 9"/>`)))
	iconCheck = ui.MustParseSVG([]byte(withIconBody(
		`<polyline points="20 6 9 17 4 12"/>`)))
	iconZap = ui.MustParseSVG([]byte(withIconBody(
		`<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>`)))
	iconSearch = ui.MustParseSVG([]byte(withIconBody(
		`<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>`)))
	iconPanel = ui.MustParseSVG([]byte(withIconBody(
		`<rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="9" y1="3" x2="9" y2="21"/>`)))
	iconMinimize = ui.MustParseSVG([]byte(withIconBody(
		`<line x1="5" y1="12" x2="19" y2="12"/>`)))
	iconMaximize = ui.MustParseSVG([]byte(withIconBody(
		`<rect x="5" y="5" width="14" height="14" rx="1" ry="1"/>`)))
)

func withIconBody(body string) string {
	return fmt.Sprintf(iconTemplate, body)
}

// cmdIcon returns the icon a command asked for, nil when unknown.
func cmdIcon(name string) *ui.SVG {
	switch name {
	case "file":
		return iconFile
	case "document":
		return iconDocument
	case "computer":
		return iconComputer
	}
	return nil
}
