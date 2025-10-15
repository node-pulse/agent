package cmd

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/node-pulse/agent/cmd/themes"
	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/metrics"
	"github.com/node-pulse/agent/internal/report"
	"github.com/spf13/cobra"
)

// Get theme for easy access
var theme = themes.Current

// viewCmd represents the view command
var viewCmd = &cobra.Command{
	Use:   "view",
	Short: "View live metrics in terminal UI",
	Long:  `Displays real-time server metrics in a beautiful terminal dashboard.`,
	RunE:  runView,
}

func init() {
	rootCmd.AddCommand(viewCmd)
}

func runView(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create sender to check buffer status
	sender, _ := report.NewSender(cfg)

	p := tea.NewProgram(
		initialModel(cfg, sender),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

type tickMsg time.Time

type model struct {
	cfg      *config.Config
	sender   *report.Sender
	report   *metrics.Report
	stats    metrics.HourlyStatsSnapshot
	err      error
	width    int
	height   int
	quitting bool
	serverID string
}

func initialModel(cfg *config.Config, sender *report.Sender) model {
	return model{
		cfg:      cfg,
		sender:   sender,
		width:    80,
		height:   24,
		serverID: cfg.Agent.ServerID,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(m.cfg.Agent.Interval),
		collectMetrics(m.serverID),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			return m, collectMetrics(m.serverID)
		}

	case tickMsg:
		return m, tea.Batch(
			collectMetrics(m.serverID),
			tickCmd(m.cfg.Agent.Interval),
		)

	case *metrics.Report:
		m.report = msg
		m.stats = metrics.GetGlobalStats().GetStats()
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
		return lipgloss.NewStyle().
			Foreground(theme.Success).
			Bold(true).
			Render("✓ Dashboard closed\n")
	}

	return m.renderDashboard()
}

func (m model) renderDashboard() string {
	// ASCII Art Logo
	logo := `
 ███╗   ██╗ ██████╗ ██████╗ ███████╗██████╗ ██╗   ██╗██╗     ███████╗███████╗
 ████╗  ██║██╔═══██╗██╔══██╗██╔════╝██╔══██╗██║   ██║██║     ██╔════╝██╔════╝
 ██╔██╗ ██║██║   ██║██║  ██║█████╗  ██████╔╝██║   ██║██║     ███████╗█████╗
 ██║╚██╗██║██║   ██║██║  ██║██╔══╝  ██╔═══╝ ██║   ██║██║     ╚════██║██╔══╝
 ██║ ╚████║╚██████╔╝██████╔╝███████╗██║     ╚██████╔╝███████╗███████║███████╗
 ╚═╝  ╚═══╝ ╚═════╝ ╚═════╝ ╚══════╝╚═╝      ╚═════╝ ╚══════╝╚══════╝╚══════╝`

	title := lipgloss.NewStyle().
		Foreground(theme.Primary).
		Render(logo)

	// Error handling
	if m.err != nil {
		errorBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Error).
			Padding(0, 1).
			Width(m.width - 4).
			Render(lipgloss.NewStyle().
				Foreground(theme.Error).
				Render(fmt.Sprintf("✗ Error: %v", m.err)))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", errorBox, m.renderFooter())
	}

	if m.report == nil {
		loading := lipgloss.NewStyle().
			Foreground(theme.Accent).
			Render("⟳ Collecting metrics...")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", loading, m.renderFooter())
	}

	// Build dashboard sections
	sections := []string{}

	// Current Metrics Section
	currentMetrics := m.renderCurrentMetrics()

	// Hourly Stats Section
	hourlyStats := m.renderHourlyStats()

	// Server Info Section
	serverInfo := m.renderServerInfo()

	// Responsive layout based on terminal width
	// If width >= 120, display side-by-side; otherwise stack vertically
	if m.width >= 120 {
		// Calculate heights and equalize them
		metricsHeight := lipgloss.Height(currentMetrics)
		serverHeight := lipgloss.Height(serverInfo)
		maxHeight := max(metricsHeight, serverHeight)

		// Add padding to equalize heights
		if metricsHeight < maxHeight {
			currentMetrics = lipgloss.NewStyle().Height(maxHeight).Render(currentMetrics)
		}
		if serverHeight < maxHeight {
			serverInfo = lipgloss.NewStyle().Height(maxHeight).Render(serverInfo)
		}

		topRow := lipgloss.JoinHorizontal(
			lipgloss.Top,
			serverInfo,
			lipgloss.NewStyle().Width(2).Render(" "),
			currentMetrics,
		)
		sections = append(sections, topRow)
		sections = append(sections, hourlyStats)
	} else {
		sections = append(sections, serverInfo)
		sections = append(sections, currentMetrics)
		sections = append(sections, hourlyStats)
	}

	// Footer
	footer := m.renderFooter()

	// Combine all sections
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		strings.Join(sections, "\n"),
		footer,
	)

	return content
}

func (m model) renderCurrentMetrics() string {
	r := m.report

	// Calculate box width based on layout mode
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Side-by-side layout: half width minus spacing
		boxWidth = (m.width / 2) - 3
	}

	// Create styled box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	var content strings.Builder

	// Section title
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Accent).
		Render("📊 Current Metrics")
	content.WriteString(header + "\n")

	// Hostname and timestamp
	metaStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
	content.WriteString(
		lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("Host: ") +
			lipgloss.NewStyle().Bold(true).Render(r.Hostname) +
			metaStyle.Render("  •  ") +
			metaStyle.Render(formatTimestamp(r.Timestamp)),
	)
	content.WriteString("\n")

	// CPU
	if r.CPU != nil {
		cpuLabel := fmt.Sprintf("CPU  %.1f%%", r.CPU.UsagePercent)
		cpuColor := getPercentColor(r.CPU.UsagePercent)
		content.WriteString(
			lipgloss.NewStyle().Foreground(cpuColor).Bold(true).Render(cpuLabel) + "\n",
		)
	} else {
		content.WriteString(renderErrorLine("CPU", "Failed to collect"))
	}

	// Memory
	if r.Memory != nil {
		memLabel := fmt.Sprintf("MEM  %.1f%% (%s / %s)",
			r.Memory.UsagePercent,
			formatBytes(r.Memory.UsedMB*1024*1024),
			formatBytes(r.Memory.TotalMB*1024*1024))
		memColor := getPercentColor(r.Memory.UsagePercent)
		content.WriteString(
			lipgloss.NewStyle().Foreground(memColor).Bold(true).Render(memLabel) + "\n",
		)
	} else {
		content.WriteString(renderErrorLine("MEM", "Failed to collect"))
	}

	// Network
	if r.Network != nil {
		upIcon := lipgloss.NewStyle().Foreground(theme.Success).Render("↑")
		downIcon := lipgloss.NewStyle().Foreground(theme.Accent).Render("↓")
		content.WriteString(fmt.Sprintf("%s Upload   %s\n", upIcon, formatBytes(r.Network.UploadBytes)))
		content.WriteString(fmt.Sprintf("%s Download %s\n", downIcon, formatBytes(r.Network.DownloadBytes)))
	} else {
		content.WriteString(renderErrorLine("NET", "Failed to collect"))
	}

	// Uptime
	if r.Uptime != nil {
		uptimeIcon := lipgloss.NewStyle().Foreground(theme.Primary).Render("⏱")
		content.WriteString(fmt.Sprintf("%s Uptime   %.1f days\n", uptimeIcon, r.Uptime.Days))
	}

	return boxStyle.Render(content.String())
}

func (m model) renderHourlyStats() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(m.width - 4)

	var content strings.Builder

	// Section title
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Success).
		Render("📈 Current Hour Stats")
	content.WriteString(header + "\n")

	stats := m.stats

	// Collections row
	content.WriteString(renderStatLine("Collections", fmt.Sprintf("%d", stats.CollectionCount)))
	content.WriteString(renderStatLine("Success", fmt.Sprintf("%d", stats.SuccessCount)))
	content.WriteString(renderStatLine("Failed", fmt.Sprintf("%d", stats.FailedCount)))

	// Averages
	if stats.CollectionCount > 0 {
		content.WriteString(renderStatLine("Avg CPU", fmt.Sprintf("%.1f%%", stats.AvgCPU)))
		content.WriteString(renderStatLine("Avg Memory", fmt.Sprintf("%.1f%%", stats.AvgMemory)))
		content.WriteString(renderStatLine("Total Upload", formatBytes(stats.TotalUpload)))
		content.WriteString(renderStatLine("Total Download", formatBytes(stats.TotalDownload)))
	} else {
		content.WriteString(lipgloss.NewStyle().
			Foreground(theme.TextMuted).
			Render("No data collected this hour yet"))
	}

	return boxStyle.Render(strings.TrimRight(content.String(), "\n"))
}

func (m model) renderServerInfo() string {
	// Calculate box width based on layout mode
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Side-by-side layout: half width minus spacing
		boxWidth = (m.width / 2) - 3
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	var content strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Primary).
		Render("🖥  Server Info")
	content.WriteString(header + "\n")

	// System info
	if m.report != nil && m.report.SystemInfo != nil {
		sys := m.report.SystemInfo
		content.WriteString(renderStatLine("Distro", fmt.Sprintf("%s %s", sys.Distro, sys.DistroVer)))
		content.WriteString(renderStatLine("Kernel", sys.KernelVer))
		content.WriteString(renderStatLine("Arch", fmt.Sprintf("%s (%d cores)", sys.Architecture, sys.CPUCores)))
	}

	// Truncate endpoint if too long
	endpoint := m.cfg.Server.Endpoint
	if len(endpoint) > 30 {
		endpoint = endpoint[:27] + "..."
	}

	content.WriteString(renderStatLine("Endpoint", endpoint))
	content.WriteString(renderStatLine("Interval", m.cfg.Agent.Interval.String()))
	content.WriteString(renderStatLine("Timeout", m.cfg.Server.Timeout.String()))

	runningTime := time.Since(m.stats.StartTime)
	content.WriteString(renderStatLine("Running", formatDuration(runningTime)))

	return boxStyle.Render(strings.TrimRight(content.String(), "\n"))
}

func (m model) renderFooter() string {
	keys := lipgloss.NewStyle().Foreground(theme.TextMuted).Render(
		"[q] quit • [r] refresh • Updates every " + m.cfg.Agent.Interval.String(),
	)
	return keys
}

// Helper functions

func renderStatLine(label, value string) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Width(16)
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))
	return labelStyle.Render(label+":") + " " + valueStyle.Render(value) + "\n"
}

func renderErrorLine(label, message string) string {
	return lipgloss.NewStyle().
		Foreground(theme.Error).
		Render(fmt.Sprintf("%s: %s\n", label, message))
}

func getPercentColor(percent float64) lipgloss.Color {
	if percent < 60 {
		return theme.Success
	} else if percent < 80 {
		return theme.Warning
	}
	return theme.Error
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
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatTimestamp(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("15:04:05")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func collectMetrics(serverID string) tea.Cmd {
	return func() tea.Msg {
		report, err := metrics.Collect(serverID)
		if err != nil {
			return err
		}
		return report
	}
}
