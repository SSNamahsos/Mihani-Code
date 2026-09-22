package ui

import "github.com/charmbracelet/lipgloss"

// Dark palette mirroring opencode's signature look: near-black neutral grays
// with a soft peach-orange primary accent.
var (
	colAccent = lipgloss.Color("#fab283") // primary — opencode peach orange
	colBlue   = lipgloss.Color("#5c9cf5") // secondary
	colText   = lipgloss.Color("#eeeeee")
	colBright = lipgloss.Color("#ffffff")
	colDim    = lipgloss.Color("#808080") // textMuted
	colFaint  = lipgloss.Color("#606060") // borderActive gray
	colBorder = lipgloss.Color("#3c3c3c") // borderSubtle
	colGreen  = lipgloss.Color("#7fd88f")
	colRed    = lipgloss.Color("#e06c75")
	colAmber  = lipgloss.Color("#f5a742")
	colPurple = lipgloss.Color("#9d7cd8")
	colCyan   = lipgloss.Color("#56b6c2")
)

type mode struct {
	name        string
	description string
	color       lipgloss.Color
}

var modes = []mode{
	{name: "build", description: "Make changes directly in your workspace", color: colAmber},
	{name: "plan", description: "Explore the task and propose an implementation", color: colBlue},
	{name: "research", description: "Investigate code, docs, and options", color: colPurple},
	{name: "ask", description: "Get explanations without changing files", color: colGreen},
}

func currentMode(index int) mode { return modes[index%len(modes)] }

// plainUI switches the whole UI to ASCII borders + a plain spinner for
// terminals whose font lacks Unicode box-drawing / braille glyphs (they would
// otherwise render as "?"). Default off; toggled in settings or via
// "plain_ui": true in config.json.
var plainUI bool

// rtlDisplay enables Persian/Arabic support on display: letters are shaped
// into their joined forms and bidi runs are reordered for terminals without
// native bidi. Default on — it only affects lines that contain RTL script.
// Toggle with /rtl or "rtl_display": false in config.json.
var rtlDisplay = true

// boxBorder is the standard rounded border, or an ASCII one in plain mode.
func boxBorder() lipgloss.Border {
	if plainUI {
		return lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|",
			TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	return lipgloss.RoundedBorder()
}

// boxBorderDouble is the double border, or an ASCII one in plain mode.
func boxBorderDouble() lipgloss.Border {
	if plainUI {
		return lipgloss.Border{Top: "=", Bottom: "=", Left: "#", Right: "#",
			TopLeft: "#", TopRight: "#", BottomLeft: "#", BottomRight: "#"}
	}
	return lipgloss.DoubleBorder()
}

// spinFrame returns the spinner glyph for the given tick. Braille frames are
// the prettiest but the least widely supported, so plain mode falls back to
// classic ASCII characters.
func spinFrame(n int) string {
	if plainUI {
		return []string{"|", "/", "-", "\\"}[n%4]
	}
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[n%10]
}
