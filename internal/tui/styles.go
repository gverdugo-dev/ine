package tui

import "github.com/charmbracelet/lipgloss"

// The palette is the one www.ine.es uses: the burgundy of the logo and the
// headings, the teal of the navigation bar, its dark variant and the pale mint
// of the highlighted panels. Each colour has a lighter twin for dark terminals,
// where the original burgundy and dark teal would not read.
var (
	burgundy = lipgloss.AdaptiveColor{Light: "#9E1834", Dark: "#D9536F"}
	teal     = lipgloss.AdaptiveColor{Light: "#457E76", Dark: "#7DB8AF"}
	darkTeal = lipgloss.AdaptiveColor{Light: "#203C37", Dark: "#DDEEEC"}
	mint     = lipgloss.AdaptiveColor{Light: "#DDEEEC", Dark: "#335B55"}
	muted    = lipgloss.AdaptiveColor{Light: "#5F6B69", Dark: "#9FB3B0"}
	white    = lipgloss.Color("#FFFFFF")
)

var (
	brandStyle   = lipgloss.NewStyle().Foreground(white).Background(lipgloss.Color("#9E1834")).Bold(true).Padding(0, 1)
	barStyle     = lipgloss.NewStyle().Foreground(white).Background(lipgloss.Color("#457E76")).Padding(0, 1)
	titleStyle   = lipgloss.NewStyle().Foreground(burgundy).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	okStyle      = lipgloss.NewStyle().Foreground(teal).Bold(true)
	failStyle    = lipgloss.NewStyle().Foreground(burgundy).Bold(true)
	panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(teal).Padding(0, 1)
	checkOn      = lipgloss.NewStyle().Foreground(burgundy).Bold(true).Render("●")
	checkOff     = lipgloss.NewStyle().Foreground(muted).Render("○")
	listTitle    = lipgloss.NewStyle().Foreground(white).Background(lipgloss.Color("#457E76")).Padding(0, 1)
	statusMsgSty = lipgloss.NewStyle().Foreground(darkTeal)
)
