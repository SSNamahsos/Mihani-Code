package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// Refined dark palette (tokyo-night inspired).
var (
	colAccent = lipgloss.Color("#7aa2f7")
	colText   = lipgloss.Color("#c0caf5")
	colBright = lipgloss.Color("#e6ecfd")
	colDim    = lipgloss.Color("#565f89")
	colFaint  = lipgloss.Color("#3b4261")
	colBorder = lipgloss.Color("#2a3150")
	colGreen  = lipgloss.Color("#9ece6a")
	colRed    = lipgloss.Color("#f7768e")
	colAmber  = lipgloss.Color("#e0af68")
	colPurple = lipgloss.Color("#bb9af7")
	colCyan   = lipgloss.Color("#7dcfff")
)

type mode struct {
	name        string
	description string
	color       lipgloss.Color
	provider    string // which provider this mode runs on (tools-aware routing)
}

var modes = []mode{
	{name: "build", description: "Make changes directly in your workspace", color: colAmber, provider: config.BuiltinPrimary},
	{name: "plan", description: "Explore the task and propose an implementation", color: colAccent, provider: config.BuiltinSecondary},
	{name: "research", description: "Investigate code, docs, and options", color: colPurple, provider: config.BuiltinSecondary},
	{name: "ask", description: "Get explanations without changing files", color: colGreen, provider: config.BuiltinSecondary},
}

func currentMode(index int) mode { return modes[index%len(modes)] }

// plainUI switches the whole UI to ASCII borders + a plain spinner for
// terminals whose font lacks Unicode box-drawing / braille glyphs (they would
// otherwise render as "?"). Default off; toggled in settings or via
// "plain_ui": true in config.json.
var plainUI bool

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
