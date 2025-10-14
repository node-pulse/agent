package cmd

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/metrics"
	"github.com/spf13/cobra"
)

// viewCmd represents the view command
var viewCmd = &cobra.Command{
	Use:   "view",
	Short: "View live metrics in terminal UI",
	Long:  `Displays real-time server metrics in a terminal user interface.`,
	RunE:  runView,
}

func init() {
	rootCmd.AddCommand(viewCmd)
}

func runView(cmd *cobra.Command, args []string) error {
	// Load configuration for interval
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	p := tea.NewProgram(initialModel(cfg.Agent.Interval))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

type tickMsg time.Time

type model struct {
	report   *metrics.Report
	err      error
	interval time.Duration
	quitting bool
}

func initialModel(interval time.Duration) model {
	return model{
		interval: interval,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(m.interval),
		collectMetrics(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tickMsg:
		return m, tea.Batch(
			collectMetrics(),
			tickCmd(m.interval),
		)

	case *metrics.Report:
		m.report = msg
		m.err = nil
		return m, nil

	case error:
		m.err = msg
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var s string

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)
	s += titleStyle.Render("NodePulse Agent - Live Metrics") + "\n\n"

	// Metrics display
	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
		s += errorStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n"
	} else if m.report != nil {
		s += renderMetrics(m.report)
	} else {
		s += "Collecting metrics...\n"
	}

	// Footer
	s += "\n"
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	s += footerStyle.Render(fmt.Sprintf("Press 'q' to quit | Refresh: %s", m.interval))

	return s
}

func renderMetrics(r *metrics.Report) string {
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Width(20)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	var s string

	// Hostname
	s += labelStyle.Render("Hostname:") + " " + valueStyle.Render(r.Hostname) + "\n"
	s += labelStyle.Render("Timestamp:") + " " + valueStyle.Render(r.Timestamp) + "\n\n"

	// CPU
	if r.CPU != nil {
		s += labelStyle.Render("CPU Usage:") + " " +
			valueStyle.Render(fmt.Sprintf("%.2f%%", r.CPU.UsagePercent)) + "\n"
	} else {
		s += labelStyle.Render("CPU Usage:") + " " +
			errorStyle.Render("Failed to collect") + "\n"
	}

	// Memory
	if r.Memory != nil {
		s += labelStyle.Render("Memory Usage:") + " " +
			valueStyle.Render(fmt.Sprintf("%.2f%% (%d MB / %d MB)",
				r.Memory.UsagePercent, r.Memory.UsedMB, r.Memory.TotalMB)) + "\n"
	} else {
		s += labelStyle.Render("Memory Usage:") + " " +
			errorStyle.Render("Failed to collect") + "\n"
	}

	// Network
	if r.Network != nil {
		s += labelStyle.Render("Network Upload:") + " " +
			valueStyle.Render(fmt.Sprintf("%s", formatBytes(r.Network.UploadBytes))) + "\n"
		s += labelStyle.Render("Network Download:") + " " +
			valueStyle.Render(fmt.Sprintf("%s", formatBytes(r.Network.DownloadBytes))) + "\n"
	} else {
		s += labelStyle.Render("Network:") + " " +
			errorStyle.Render("Failed to collect") + "\n"
	}

	// Uptime
	if r.Uptime != nil {
		s += labelStyle.Render("Uptime:") + " " +
			valueStyle.Render(fmt.Sprintf("%.2f days", r.Uptime.Days)) + "\n"
	} else {
		s += labelStyle.Render("Uptime:") + " " +
			errorStyle.Render("Failed to collect") + "\n"
	}

	return s
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func collectMetrics() tea.Cmd {
	return func() tea.Msg {
		report, err := metrics.Collect()
		if err != nil {
			return err
		}
		return report
	}
}
