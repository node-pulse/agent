package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/node-pulse/agent/internal/config"
	"github.com/node-pulse/agent/internal/metrics"
	"github.com/node-pulse/agent/internal/report"
	"github.com/spf13/cobra"
)

// Color palette
var (
	primaryColor   = lipgloss.Color("#7C3AED") // Purple
	successColor   = lipgloss.Color("#10B981") // Green
	warningColor   = lipgloss.Color("#F59E0B") // Orange
	errorColor     = lipgloss.Color("#EF4444") // Red
	accentColor    = lipgloss.Color("#06B6D4") // Cyan
	mutedColor     = lipgloss.Color("#6B7280") // Gray
	bgColor        = lipgloss.Color("#1F2937") // Dark bg
	borderColor    = lipgloss.Color("#374151") // Border
)

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
	cfg          *config.Config
	sender       *report.Sender
	report       *metrics.Report
	stats        metrics.HourlyStatsSnapshot
	bufferStatus report.BufferStatus
	cpuProgress  progress.Model
	memProgress  progress.Model
	err          error
	width        int
	height       int
	quitting     bool
}

func initialModel(cfg *config.Config, sender *report.Sender) model {
	cpuProg := progress.New(
		progress.WithGradient(string(warningColor), string(errorColor)),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	memProg := progress.New(
		progress.WithGradient(string(accentColor), string(primaryColor)),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return model{
		cfg:         cfg,
		sender:      sender,
		cpuProgress: cpuProg,
		memProgress: memProg,
		width:       80,
		height:      24,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(m.cfg.Agent.Interval),
		collectMetrics(),
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
			return m, collectMetrics()
		}

	case tickMsg:
		return m, tea.Batch(
			collectMetrics(),
			tickCmd(m.cfg.Agent.Interval),
		)

	case *metrics.Report:
		m.report = msg
		m.stats = metrics.GetGlobalStats().GetStats()
		if m.sender != nil {
			m.bufferStatus = m.sender.GetBufferStatus()
		}
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
			Foreground(successColor).
			Bold(true).
			Render("✓ Dashboard closed\n")
	}

	return m.renderDashboard()
}

func (m model) renderDashboard() string {
	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Background(bgColor).
		Padding(0, 2).
		MarginBottom(1).
		Render("⚡ NodePulse Agent Dashboard")

	// Error handling
	if m.err != nil {
		errorBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(errorColor).
			Padding(1, 2).
			Width(m.width - 4).
			Render(lipgloss.NewStyle().
				Foreground(errorColor).
				Render(fmt.Sprintf("✗ Error: %v", m.err)))
		return lipgloss.JoinVertical(lipgloss.Left, title, errorBox, m.renderFooter())
	}

	if m.report == nil {
		loading := lipgloss.NewStyle().
			Foreground(accentColor).
			Render("⟳ Collecting metrics...")
		return lipgloss.JoinVertical(lipgloss.Left, title, loading, m.renderFooter())
	}

	// Build dashboard sections
	sections := []string{}

	// Current Metrics Section
	currentMetrics := m.renderCurrentMetrics()
	sections = append(sections, currentMetrics)

	// Hourly Stats Section
	hourlyStats := m.renderHourlyStats()
	sections = append(sections, hourlyStats)

	// Two-column layout for buffer and agent info
	bufferInfo := m.renderBufferStatus()
	agentInfo := m.renderAgentInfo()

	bottomRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		bufferInfo,
		lipgloss.NewStyle().Width(2).Render(" "),
		agentInfo,
	)
	sections = append(sections, bottomRow)

	// Footer
	footer := m.renderFooter()

	// Combine all sections
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		strings.Join(sections, "\n\n"),
		"",
		footer,
	)

	return content
}

func (m model) renderCurrentMetrics() string {
	r := m.report

	// Create styled box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(m.width - 4)

	var content strings.Builder

	// Section title
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Render("📊 Current Metrics")
	content.WriteString(header + "\n\n")

	// Hostname and timestamp
	metaStyle := lipgloss.NewStyle().Foreground(mutedColor)
	content.WriteString(
		lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("Host: ") +
			lipgloss.NewStyle().Bold(true).Render(r.Hostname) +
			metaStyle.Render("  •  ") +
			metaStyle.Render(formatTimestamp(r.Timestamp)),
	)
	content.WriteString("\n\n")

	// CPU with progress bar
	if r.CPU != nil {
		cpuPercent := r.CPU.UsagePercent / 100.0
		cpuBar := m.cpuProgress.ViewAs(cpuPercent)
		cpuLabel := fmt.Sprintf("CPU  %5.1f%%", r.CPU.UsagePercent)
		cpuColor := getPercentColor(r.CPU.UsagePercent)
		content.WriteString(
			lipgloss.NewStyle().Foreground(cpuColor).Bold(true).Render(cpuLabel) +
				"  " + cpuBar + "\n",
		)
	} else {
		content.WriteString(renderErrorLine("CPU", "Failed to collect"))
	}

	// Memory with progress bar
	if r.Memory != nil {
		memPercent := r.Memory.UsagePercent / 100.0
		memBar := m.memProgress.ViewAs(memPercent)
		memLabel := fmt.Sprintf("MEM  %5.1f%%", r.Memory.UsagePercent)
		memColor := getPercentColor(r.Memory.UsagePercent)
		memInfo := fmt.Sprintf(" %s / %s",
			formatBytes(r.Memory.UsedMB*1024*1024),
			formatBytes(r.Memory.TotalMB*1024*1024))
		content.WriteString(
			lipgloss.NewStyle().Foreground(memColor).Bold(true).Render(memLabel) +
				"  " + memBar +
				lipgloss.NewStyle().Foreground(mutedColor).Render(memInfo) + "\n",
		)
	} else {
		content.WriteString(renderErrorLine("MEM", "Failed to collect"))
	}

	content.WriteString("\n")

	// Network
	if r.Network != nil {
		upIcon := lipgloss.NewStyle().Foreground(successColor).Render("↑")
		downIcon := lipgloss.NewStyle().Foreground(accentColor).Render("↓")
		content.WriteString(fmt.Sprintf("%s Upload   %s\n", upIcon, formatBytes(r.Network.UploadBytes)))
		content.WriteString(fmt.Sprintf("%s Download %s\n", downIcon, formatBytes(r.Network.DownloadBytes)))
	} else {
		content.WriteString(renderErrorLine("NET", "Failed to collect"))
	}

	// Uptime
	if r.Uptime != nil {
		uptimeIcon := lipgloss.NewStyle().Foreground(primaryColor).Render("⏱")
		content.WriteString(fmt.Sprintf("\n%s Uptime   %.1f days", uptimeIcon, r.Uptime.Days))
	}

	return boxStyle.Render(content.String())
}

func (m model) renderHourlyStats() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(m.width - 4)

	var content strings.Builder

	// Section title
	hour := time.Now().Format("15:00")
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(successColor).
		Render(fmt.Sprintf("📈 Current Hour Stats (%s)", hour))
	content.WriteString(header + "\n\n")

	stats := m.stats

	// Collections row
	content.WriteString(renderStatLine("Collections", fmt.Sprintf("%d", stats.CollectionCount)))
	content.WriteString(renderStatLine("Success", fmt.Sprintf("%d", stats.SuccessCount)))
	content.WriteString(renderStatLine("Failed", fmt.Sprintf("%d", stats.FailedCount)))

	content.WriteString("\n")

	// Averages
	if stats.CollectionCount > 0 {
		content.WriteString(renderStatLine("Avg CPU", fmt.Sprintf("%.1f%%", stats.AvgCPU)))
		content.WriteString(renderStatLine("Avg Memory", fmt.Sprintf("%.1f%%", stats.AvgMemory)))
		content.WriteString("\n")
		content.WriteString(renderStatLine("Total Upload", formatBytes(stats.TotalUpload)))
		content.WriteString(renderStatLine("Total Download", formatBytes(stats.TotalDownload)))
	} else {
		content.WriteString(lipgloss.NewStyle().
			Foreground(mutedColor).
			Render("No data collected this hour yet"))
	}

	return boxStyle.Render(content.String())
}

func (m model) renderBufferStatus() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width((m.width / 2) - 3)

	var content strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(warningColor).
		Render("💾 Buffer Status")
	content.WriteString(header + "\n\n")

	if !m.bufferStatus.HasBuffered {
		content.WriteString(lipgloss.NewStyle().
			Foreground(successColor).
			Render("✓ No buffered reports"))
	} else {
		content.WriteString(renderStatLine("Buffered Files", fmt.Sprintf("%d", m.bufferStatus.FileCount)))
		content.WriteString(renderStatLine("Reports Queued", fmt.Sprintf("%d", m.bufferStatus.ReportCount)))
		content.WriteString(renderStatLine("Size", fmt.Sprintf("%d KB", m.bufferStatus.TotalSizeKB)))
		if !m.bufferStatus.OldestFile.IsZero() {
			content.WriteString(renderStatLine("Oldest", m.bufferStatus.OldestFile.Format("15:04")))
		}
	}

	return boxStyle.Render(content.String())
}

func (m model) renderAgentInfo() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width((m.width / 2) - 3)

	var content strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Render("⚙️  Agent Info")
	content.WriteString(header + "\n\n")

	// Truncate endpoint if too long
	endpoint := m.cfg.Server.Endpoint
	if len(endpoint) > 35 {
		endpoint = endpoint[:32] + "..."
	}

	content.WriteString(renderStatLine("Endpoint", endpoint))
	content.WriteString(renderStatLine("Interval", m.cfg.Agent.Interval.String()))
	content.WriteString(renderStatLine("Timeout", m.cfg.Server.Timeout.String()))

	runningTime := time.Since(m.stats.StartTime)
	content.WriteString(renderStatLine("Running", formatDuration(runningTime)))

	return boxStyle.Render(content.String())
}

func (m model) renderFooter() string {
	keys := lipgloss.NewStyle().Foreground(mutedColor).Render(
		"[q] quit • [r] refresh • Updates every " + m.cfg.Agent.Interval.String(),
	)
	return keys
}

// Helper functions

func renderStatLine(label, value string) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(16)
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))
	return labelStyle.Render(label+":") + " " + valueStyle.Render(value) + "\n"
}

func renderErrorLine(label, message string) string {
	return lipgloss.NewStyle().
		Foreground(errorColor).
		Render(fmt.Sprintf("%s: %s\n", label, message))
}

func getPercentColor(percent float64) lipgloss.Color {
	if percent < 60 {
		return successColor
	} else if percent < 80 {
		return warningColor
	}
	return errorColor
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

func collectMetrics() tea.Cmd {
	return func() tea.Msg {
		report, err := metrics.Collect()
		if err != nil {
			return err
		}
		return report
	}
}
