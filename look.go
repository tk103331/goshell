package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// look is the color of the app's chrome, kept apart from the controls'
// theme: the wash under the sidebar and the title bar, the card the
// terminal sits on, and the states of the rows that fill them. It follows
// kero's palette, which keeps the shell quiet and lets the terminal be the
// brightest thing in the window.
type look struct {
	dark bool

	wash       ui.Color // behind the sidebar, the title bar and around the card
	card       ui.Color // the terminal's card
	cardBorder ui.Color
	shadow     ui.Color

	text      ui.Color
	textMuted ui.Color // section headers, secondary text
	textFaint ui.Color // counts, hints

	rowHover    ui.Color
	rowSelected ui.Color
	rowSelEdge  ui.Color
	chip        ui.Color
	divider     ui.Color
	accent      ui.Color
	danger      ui.Color
	attention   ui.Color
	success     ui.Color

	accentSoft    ui.Color
	dangerSoft    ui.Color
	attentionSoft ui.Color
	successSoft   ui.Color
}

var lightLook = look{
	wash:       ui.Hex("#eceef4"),
	card:       ui.Hex("#ffffff"),
	cardBorder: ui.RGBA(0, 0, 0, 0.07),
	shadow:     ui.RGBA(20, 20, 50, 0.07),

	text:      ui.Hex("#1d1d1f"),
	textMuted: ui.Hex("#6e6e78"),
	textFaint: ui.Hex("#a3a3ad"),

	rowHover:    ui.RGBA(0, 0, 0, 0.045),
	rowSelected: ui.Hex("#ffffff"),
	rowSelEdge:  ui.RGBA(0, 0, 0, 0.06),
	chip:        ui.RGBA(0, 0, 0, 0.045),
	divider:     ui.RGBA(0, 0, 0, 0.06),
	accent:      ui.Hex("#3b6cf6"),
	danger:      ui.Hex("#d93a35"),
	attention:   ui.Hex("#c27b0f"),
	success:     ui.Hex("#2fa84f"),

	accentSoft:    ui.RGBA(59, 108, 246, 0.09),
	dangerSoft:    ui.RGBA(217, 58, 53, 0.09),
	attentionSoft: ui.RGBA(194, 123, 15, 0.11),
	successSoft:   ui.RGBA(47, 168, 79, 0.09),
}

var darkLook = look{
	dark:       true,
	wash:       ui.Hex("#141418"),
	card:       ui.Hex("#232328"),
	cardBorder: ui.RGBA(255, 255, 255, 0.08),
	shadow:     ui.RGBA(0, 0, 0, 0.35),

	text:      ui.Hex("#ececf0"),
	textMuted: ui.Hex("#a9a9b4"),
	textFaint: ui.Hex("#83838f"),

	rowHover:    ui.RGBA(255, 255, 255, 0.05),
	rowSelected: ui.RGBA(255, 255, 255, 0.10),
	rowSelEdge:  ui.RGBA(255, 255, 255, 0.06),
	chip:        ui.RGBA(255, 255, 255, 0.07),
	divider:     ui.RGBA(255, 255, 255, 0.07),
	accent:      ui.Hex("#6b93ff"),
	danger:      ui.Hex("#ff6961"),
	attention:   ui.Hex("#f0a33a"),
	success:     ui.Hex("#4fd17a"),

	accentSoft:    ui.RGBA(107, 147, 255, 0.12),
	dangerSoft:    ui.RGBA(255, 105, 97, 0.12),
	attentionSoft: ui.RGBA(240, 163, 58, 0.14),
	successSoft:   ui.RGBA(79, 209, 122, 0.12),
}

// lookFor is the chrome color of the window c draws.
func lookFor(c *ui.Context) *look {
	if c.Theme().Dark {
		return &darkLook
	}
	return &lightLook
}

// themeLight and themeDark color the controls and dialogs on the card's
// face; the view paints the wash itself.
var (
	themeLight = func() *ui.Theme {
		t := ui.LightTheme()
		t.Background = lightLook.card
		t.Surface = lightLook.card
		t.SurfaceHover = ui.Hex("#f4f5f9")
		t.SurfacePressed = ui.Hex("#e7e8ee")
		t.Border = ui.RGBA(0, 0, 0, 0.10)
		t.Text = lightLook.text
		t.TextMuted = lightLook.textMuted
		t.Accent = ui.Hex("#3b6cf6")
		t.AccentHover = ui.Hex("#2f5ddb")
		t.AccentPressed = ui.Hex("#254bb4")
		t.Danger = ui.Hex("#d93a35")
		t.Warning = ui.Hex("#c27b0f")
		t.Success = ui.Hex("#2fa84f")
		t.Selection = ui.RGBA(59, 108, 246, 0.22)
		t.Focus = ui.RGBA(59, 108, 246, 0.55)
		t.Scrollbar = ui.RGBA(0, 0, 0, 0.20)
		t.Radius = 8
		return t
	}()
	themeDark = func() *ui.Theme {
		t := ui.DarkTheme()
		t.Background = darkLook.card
		t.Surface = darkLook.card
		t.SurfaceHover = ui.Hex("#2c2c33")
		t.SurfacePressed = ui.Hex("#35353d")
		t.Border = ui.RGBA(255, 255, 255, 0.10)
		t.Text = darkLook.text
		t.TextMuted = darkLook.textMuted
		t.Accent = ui.Hex("#6b93ff")
		t.AccentHover = ui.Hex("#8aabff")
		t.AccentPressed = ui.Hex("#4f7cf0")
		t.Danger = ui.Hex("#ff6961")
		t.Warning = ui.Hex("#f0a33a")
		t.Success = ui.Hex("#4fd17a")
		t.Selection = ui.RGBA(107, 147, 255, 0.35)
		t.Focus = ui.RGBA(107, 147, 255, 0.60)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.22)
		t.Radius = 8
		return t
	}()
)

// termLight and termDark are the terminals' colors until the user picks a
// Ghostty theme: the card's face, so that the terminal and its card read as
// one surface, and kero's palette for the text.
var (
	termLight = &terminal.Theme{
		Foreground:    ui.Hex("#1d1d1f"),
		Background:    lightLook.card,
		Cursor:        ui.Hex("#1d1d1f"),
		CursorText:    ui.Hex("#ffffff"),
		Selection:     ui.Hex("#cfdcfb"),
		SelectionText: ui.Hex("#1d1d1f"),
		Palette: [16]ui.Color{
			ui.Hex("#1d1d1f"), ui.Hex("#d3302f"), ui.Hex("#22863a"), ui.Hex("#b08800"),
			ui.Hex("#2f5bd8"), ui.Hex("#a033c4"), ui.Hex("#0f8199"), ui.Hex("#8e8e93"),
			ui.Hex("#5f5f66"), ui.Hex("#ef4444"), ui.Hex("#2fa84f"), ui.Hex("#d4a017"),
			ui.Hex("#3b82f6"), ui.Hex("#c04ee0"), ui.Hex("#14a3bf"), ui.Hex("#b4b4bb"),
		},
	}
	termDark = &terminal.Theme{
		Foreground:    ui.Hex("#e6e6ea"),
		Background:    darkLook.card,
		Cursor:        ui.Hex("#e6e6ea"),
		CursorText:    ui.Hex("#161618"),
		Selection:     ui.Hex("#2f4170"),
		SelectionText: ui.Hex("#ffffff"),
		Palette: [16]ui.Color{
			ui.Hex("#3a3a40"), ui.Hex("#ff6b64"), ui.Hex("#4fd17a"), ui.Hex("#f2c14e"),
			ui.Hex("#6b9bff"), ui.Hex("#d07cf0"), ui.Hex("#4cc9e0"), ui.Hex("#c9c9cf"),
			ui.Hex("#6c6c78"), ui.Hex("#ff8a84"), ui.Hex("#74e397"), ui.Hex("#ffd479"),
			ui.Hex("#8fb3ff"), ui.Hex("#e09cf7"), ui.Hex("#7fdcec"), ui.Hex("#f5f5f7"),
		},
	}
)

// dialogHeading is the title of a dialog, set apart from its fields.
func dialogHeading(c *ui.Context, title string) {
	lk := lookFor(c)
	ui.Text(c, title).FontSize(16).FontWeight(650).TextColor(lk.text).SingleLine()
}
