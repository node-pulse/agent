package themes

import "github.com/charmbracelet/lipgloss"

// Theme holds all color definitions for the UI
// This allows easy theme switching in the future by creating different theme functions
// (e.g., Dark(), Light(), Solarized(), etc.) and letting users choose via config or flag
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
// TODO: In the future, this can be set based on user preference from config
// Example usage:
//   - Current = Default()
//   - Current = Light()
//   - Current = Solarized()
//   - Current = LoadFromConfig()
var Current = Default()

// Future theme examples:
//
// func Light() Theme {
//     return Theme{
//         Primary:       lipgloss.Color("#7C3AED"),
//         Success:       lipgloss.Color("#059669"),
//         Warning:       lipgloss.Color("#D97706"),
//         Error:         lipgloss.Color("#DC2626"),
//         Accent:        lipgloss.Color("#0891B2"),
//         TextPrimary:   lipgloss.Color("#111827"), // Dark text for light bg
//         TextSecondary: lipgloss.Color("#4B5563"),
//         TextMuted:     lipgloss.Color("#9CA3AF"),
//         Background:    lipgloss.Color("#F9FAFB"), // Light bg
//         Border:        lipgloss.Color("#D1D5DB"),
//     }
// }
//
// func Solarized() Theme {
//     return Theme{
//         Primary:       lipgloss.Color("#268BD2"),
//         Success:       lipgloss.Color("#859900"),
//         Warning:       lipgloss.Color("#B58900"),
//         Error:         lipgloss.Color("#DC322F"),
//         Accent:        lipgloss.Color("#2AA198"),
//         TextPrimary:   lipgloss.Color("#839496"),
//         TextSecondary: lipgloss.Color("#657B83"),
//         TextMuted:     lipgloss.Color("#586E75"),
//         Background:    lipgloss.Color("#002B36"),
//         Border:        lipgloss.Color("#073642"),
//     }
// }
