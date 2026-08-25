package ui

import "github.com/charmbracelet/lipgloss"

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
}

var modes = []mode{
	{name: "build", description: "Make changes directly in your workspace", color: colAmber},
	{name: "plan", description: "Explore the task and propose an implementation", color: colAccent},
	{name: "research", description: "Investigate code, docs, and options", color: colPurple},
	{name: "ask", description: "Get explanations without changing files", color: colGreen},
}

func currentMode(index int) mode { return modes[index%len(modes)] }
