package themes

import "github.com/charmbracelet/lipgloss"

// Theme holds all color definitions for the UI
type Theme struct {
	// Primary colors
	Primary   lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Accent    lipgloss.Color

	// Text colors
	TextPrimary   lipgloss.Color // Bright white/main text
	TextSecondary lipgloss.Color // Lighter gray for labels
	TextMuted     lipgloss.Color // Muted gray for help text

	// UI colors
	Background lipgloss.Color
	Border     lipgloss.Color
}

// Default returns the default color theme
func Default() Theme {
	return Theme{
		// Primary colors
		Primary: lipgloss.Color("#7C3AED"), // Purple
		Success: lipgloss.Color("#10B981"), // Green
		Warning: lipgloss.Color("#F59E0B"), // Orange
		Error:   lipgloss.Color("#EF4444"), // Red
		Accent:  lipgloss.Color("#06B6D4"), // Cyan

		// Text colors
		TextPrimary:   lipgloss.Color("#E5E7EB"), // Bright gray/white
		TextSecondary: lipgloss.Color("#9CA3AF"), // Light gray
		TextMuted:     lipgloss.Color("#6B7280"), // Muted gray

		// UI colors
		Background: lipgloss.Color("#1F2937"), // Dark bg
		Border:     lipgloss.Color("#374151"), // Border
	}
}

// Global theme instance
var Current = Default()
