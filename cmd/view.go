package cmd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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
	cfg          *config.Config
	sender       *report.Sender
	report       *metrics.Report
	stats        metrics.HourlyStatsSnapshot
	err          error
	width        int
	height       int
	quitting     bool
	serverID     string
	cpuHistory   []float64 // Last 20 CPU readings for sparkline
	memHistory   []float64 // Last 20 Memory readings for sparkline
	alerts       []string  // Recent alerts
}

func initialModel(cfg *config.Config, sender *report.Sender) model {
	return model{
		cfg:        cfg,
		sender:     sender,
		width:      80,
		height:     24,
		serverID:   cfg.Agent.ServerID,
		cpuHistory: make([]float64, 0, 20),
		memHistory: make([]float64, 0, 20),
		alerts:     make([]string, 0, 5),
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

		// Update history for sparklines
		if msg.CPU != nil {
			m.cpuHistory = append(m.cpuHistory, msg.CPU.UsagePercent)
			if len(m.cpuHistory) > 20 {
				m.cpuHistory = m.cpuHistory[1:]
			}

			// Check for CPU alert
			if msg.CPU.UsagePercent > 80 {
				m.addAlert(fmt.Sprintf("High CPU: %.1f%%", msg.CPU.UsagePercent))
			}
		}

		if msg.Memory != nil {
			m.memHistory = append(m.memHistory, msg.Memory.UsagePercent)
			if len(m.memHistory) > 20 {
				m.memHistory = m.memHistory[1:]
			}

			// Check for Memory alert
			if msg.Memory.UsagePercent > 90 {
				m.addAlert(fmt.Sprintf("High Memory: %.1f%%", msg.Memory.UsagePercent))
			}
		}

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

func (m *model) addAlert(alert string) {
	timestamp := time.Now().Format("15:04:05")
	m.alerts = append(m.alerts, fmt.Sprintf("[%s] %s", timestamp, alert))
	if len(m.alerts) > 5 {
		m.alerts = m.alerts[1:]
	}
}

func (m model) renderDashboard() string {
	// Force width to be even for consistent box alignment
	if m.width%2 != 0 {
		m.width--
	}

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

	// Core Metrics Sections
	currentMetrics := m.renderCurrentMetrics()
	serverInfo := m.renderServerInfo()

	// Monitoring sections
	trendGraphs := m.renderTrendGraphs()
	alerts := m.renderAlerts()
	agentStatus := m.renderAgentStatus()
	topProcesses := m.renderTopProcesses()

	// Responsive layout based on terminal width
	// If width >= 120, display grid layout; otherwise stack vertically
	if m.width >= 120 {
		// Row 1: Server Info and Current Metrics
		row1 := lipgloss.JoinHorizontal(
			lipgloss.Top,
			serverInfo,
			lipgloss.NewStyle().Width(1).Render(" "),
			currentMetrics,
		)

		// Row 2: Agent Status (full width)
		row2 := agentStatus

		// Row 3: Trend Graphs and Alerts
		row3 := lipgloss.JoinHorizontal(
			lipgloss.Top,
			trendGraphs,
			lipgloss.NewStyle().Width(1).Render(" "),
			alerts,
		)

		// Row 4: Top Processes (full width)
		row4 := topProcesses

		sections = append(sections, row1)
		sections = append(sections, row2)
		sections = append(sections, row3)
		sections = append(sections, row4)
	} else {
		sections = append(sections, serverInfo)
		sections = append(sections, currentMetrics)
		sections = append(sections, agentStatus)
		sections = append(sections, trendGraphs)
		sections = append(sections, alerts)
		sections = append(sections, topProcesses)
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

func (m model) renderTrendGraphs() string {
	// Calculate box width based on layout mode
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Side-by-side layout: calculate exact half width
		// Total available: m.width - 4 (margins) - 1 (space between) = m.width - 5
		// Each box gets half: (m.width - 5) / 2
		// But Width() sets content width, so subtract borders (2) and padding (2)
		boxWidth = (m.width - 5) / 2 - 4
	} else {
		// Stack view: account for borders (2) and padding (2)
		boxWidth = boxWidth - 4
		maxStackWidth := 76 // 80 - 4 for borders and padding
		if boxWidth > maxStackWidth {
			boxWidth = maxStackWidth
		}
	}

	var content strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Accent).
		Render("📈 Trend Graphs")
	content.WriteString(header + "\n")

	// CPU Trend
	if len(m.cpuHistory) > 0 {
		sparkline := generateSparkline(m.cpuHistory)
		latest := m.cpuHistory[len(m.cpuHistory)-1]
		cpuColor := getPercentColor(latest)
		content.WriteString(
			lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render("CPU: ") +
				lipgloss.NewStyle().Foreground(cpuColor).Render(sparkline) +
				lipgloss.NewStyle().Foreground(cpuColor).Bold(true).Render(fmt.Sprintf(" %.1f%%", latest)) + "\n",
		)
	} else {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render("CPU: No data yet\n"))
	}

	// Memory Trend
	if len(m.memHistory) > 0 {
		sparkline := generateSparkline(m.memHistory)
		latest := m.memHistory[len(m.memHistory)-1]
		memColor := getPercentColor(latest)
		content.WriteString(
			lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render("MEM: ") +
				lipgloss.NewStyle().Foreground(memColor).Render(sparkline) +
				lipgloss.NewStyle().Foreground(memColor).Bold(true).Render(fmt.Sprintf(" %.1f%%", latest)) + "\n",
		)
	} else {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render("MEM: No data yet\n"))
	}

	// Show timeframe
	timeframe := fmt.Sprintf("Last %d collections", len(m.cpuHistory))
	content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Italic(true).Render(timeframe))

	contentStr := strings.TrimRight(content.String(), "\n")

	// Calculate heights for both boxes and use the maximum
	trendGraphsHeight := 5 // Fixed: header + CPU + MEM + blank + timeframe
	alertsHeight := m.getAlertsContentHeight()
	maxHeight := max(trendGraphsHeight, alertsHeight)

	currentHeight := strings.Count(contentStr, "\n") + 1

	// Pad to match the maximum height
	if currentHeight < maxHeight {
		for i := currentHeight; i < maxHeight; i++ {
			contentStr += "\n"
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	return boxStyle.Render(contentStr)
}

func (m model) renderAlerts() string {
	// Calculate box width based on layout mode
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Side-by-side layout: calculate exact half width (same as trendGraphs)
		// Total available: m.width - 4 (margins) - 1 (space between) = m.width - 5
		// Each box gets half: (m.width - 5) / 2
		// But Width() sets content width, so subtract borders (2) and padding (2)
		boxWidth = (m.width - 5) / 2 - 4
	} else {
		// Stack view: account for borders (2) and padding (2)
		boxWidth = boxWidth - 4
		maxStackWidth := 76 // 80 - 4 for borders and padding
		if boxWidth > maxStackWidth {
			boxWidth = maxStackWidth
		}
	}

	var content strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Warning).
		Render("🚨 Recent Alerts")
	content.WriteString(header + "\n")

	if len(m.alerts) == 0 {
		content.WriteString(lipgloss.NewStyle().
			Foreground(theme.Success).
			Render("✓ No alerts - all metrics normal"))
	} else {
		for _, alert := range m.alerts {
			content.WriteString(lipgloss.NewStyle().
				Foreground(theme.Warning).
				Render("• " + alert + "\n"))
		}
	}

	contentStr := strings.TrimRight(content.String(), "\n")

	// Calculate heights for both boxes and use the maximum
	trendGraphsHeight := 5 // Fixed: header + CPU + MEM + blank + timeframe
	alertsHeight := m.getAlertsContentHeight()
	maxHeight := max(trendGraphsHeight, alertsHeight)

	currentHeight := strings.Count(contentStr, "\n") + 1

	// Pad to match the maximum height
	if currentHeight < maxHeight {
		for i := currentHeight; i < maxHeight; i++ {
			contentStr += "\n"
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	return boxStyle.Render(contentStr)
}

// getAlertsContentHeight returns the number of lines in the alerts box content
func (m model) getAlertsContentHeight() int {
	// Header: 1 line
	// Content: either 1 line (no alerts message) or len(alerts) lines
	if len(m.alerts) == 0 {
		return 2 // header + "no alerts" message
	}
	return 1 + len(m.alerts) // header + alert lines
}

func (m model) renderAgentStatus() string {
	// Calculate box width - match the combined width of side-by-side boxes
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Match the visual width of the two boxes above (including the space between them)
		// Two boxes: each is (m.width - 5) / 2 - 4 content + 2 borders = (m.width - 5) / 2 - 2 total
		// With 1 space between: (m.width - 5) - 2 - 2 + 1 = m.width - 8
		// So content width should be: m.width - 8 - 2 = m.width - 10
		// Made 1 character narrower for better visual alignment
		boxWidth = m.width - 11
	} else {
		// Stack view: account for borders (2) and padding (2)
		boxWidth = boxWidth - 4
		maxStackWidth := 76 // 80 - 4 for borders and padding
		if boxWidth > maxStackWidth {
			boxWidth = maxStackWidth
		}
	}

	var content strings.Builder

	// Header with version
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Primary).
		Render("🔧 Node Pulse Agent Status (v1.0.0)")
	content.WriteString(header + "\n")

	// Prepare 6 items for 2-column layout (3 rows)
	items := []struct {
		label string
		value string
	}{
		{"PID", fmt.Sprintf("%d", os.Getpid())},
	}

	items = append(items, struct {
		label string
		value string
	}{"Interval", m.cfg.Agent.Interval.String()})

	// Agent memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	agentMem := memStats.Alloc / 1024 / 1024 // Convert to MB
	items = append(items, struct {
		label string
		value string
	}{"Agent Memory", fmt.Sprintf("%d MB", agentMem)})

	items = append(items, struct {
		label string
		value string
	}{"Timeout", m.cfg.Server.Timeout.String()})

	// Config source
	configPath := cfgFile
	if configPath == "" {
		configPath = "default"
	} else if len(configPath) > 25 {
		configPath = "..." + configPath[len(configPath)-22:]
	}
	items = append(items, struct {
		label string
		value string
	}{"Config", configPath})

	runningTime := time.Since(m.stats.StartTime)
	items = append(items, struct {
		label string
		value string
	}{"Running", formatDuration(runningTime)})

	// Render in 2 columns, 3 rows
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#D1D5DB")).
		Width(16)
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F3F4F6")).
		Bold(true)

	// Calculate column width (half of content width, minus spacing)
	contentWidth := boxWidth - 4 // minus borders and padding
	colWidth := (contentWidth - 2) / 2 // minus spacing between columns

	for i := 0; i < 3; i++ {
		leftIdx := i
		rightIdx := i + 3

		// Left column
		leftLine := labelStyle.Render(items[leftIdx].label+":") + " " + valueStyle.Render(items[leftIdx].value)

		// Right column
		rightLine := labelStyle.Render(items[rightIdx].label+":") + " " + valueStyle.Render(items[rightIdx].value)

		// Combine with proper width
		leftCol := lipgloss.NewStyle().Width(colWidth).Render(leftLine)
		rightCol := lipgloss.NewStyle().Width(colWidth).Render(rightLine)

		content.WriteString(leftCol + "  " + rightCol + "\n")
	}

	// Endpoint on its own line (full width)
	endpointLine := labelStyle.Render("Endpoint:") + " " + valueStyle.Render(m.cfg.Server.Endpoint)
	content.WriteString(endpointLine + "\n")

	contentStr := strings.TrimRight(content.String(), "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	return boxStyle.Render(contentStr)
}

func (m model) renderTopProcesses() string {
	// Calculate box width - match the combined width of side-by-side boxes
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Match the visual width of the two boxes above (including the space between them)
		// Two boxes: each is (m.width - 5) / 2 - 4 content + 2 borders = (m.width - 5) / 2 - 2 total
		// With 1 space between: (m.width - 5) - 2 - 2 + 1 = m.width - 8
		// So content width should be: m.width - 8 - 2 = m.width - 10
		// Made 1 character narrower for better visual alignment
		boxWidth = m.width - 11
	} else {
		// Stack view: account for borders (2) and padding (2)
		boxWidth = boxWidth - 4
		maxStackWidth := 76 // 80 - 4 for borders and padding
		if boxWidth > maxStackWidth {
			boxWidth = maxStackWidth
		}
	}

	var content strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#22D3EE")).
		Render("▲ Top Processes")
	content.WriteString(header + "\n")

	// Get top processes
	topCPU, topMem := getTopProcesses()

	// Calculate content width (box width minus borders and padding)
	contentWidth := boxWidth - 4
	colWidth := (contentWidth - 2) / 2 // minus spacing between columns

	// Build CPU column (without colors - we'll apply them later)
	var cpuCol strings.Builder
	cpuCol.WriteString("CPU:\n")
	if len(topCPU) > 0 {
		for i, proc := range topCPU {
			if i >= 3 {
				break
			}
			// Format CPU time in seconds (jiffies / 100 = seconds on most systems)
			cpuSecs := proc.usage
			var cpuStr string
			if cpuSecs < 60 {
				cpuStr = fmt.Sprintf("%.0fs", cpuSecs)
			} else if cpuSecs < 3600 {
				cpuStr = fmt.Sprintf("%.1fm", cpuSecs/60)
			} else {
				cpuStr = fmt.Sprintf("%.1fh", cpuSecs/3600)
			}

			// Truncate process name if too long
			displayName := proc.name
			if len(displayName) > 15 {
				displayName = displayName[:12] + "..."
			}

			cpuCol.WriteString(fmt.Sprintf("  %s (%s)\n", displayName, cpuStr))
		}
	} else {
		cpuCol.WriteString("  No data available\n")
	}

	// Build Memory column (without colors - we'll apply them later)
	var memCol strings.Builder
	memCol.WriteString("Memory:\n")
	if len(topMem) > 0 {
		for i, proc := range topMem {
			if i >= 3 {
				break
			}
			// Truncate process name if too long
			displayName := proc.name
			if len(displayName) > 15 {
				displayName = displayName[:12] + "..."
			}

			memCol.WriteString(fmt.Sprintf("  %s (%s)\n", displayName, formatBytes(uint64(proc.usage*1024*1024))))
		}
	} else {
		memCol.WriteString("  No data available\n")
	}

	// Split columns into lines
	cpuLines := strings.Split(strings.TrimRight(cpuCol.String(), "\n"), "\n")
	memLines := strings.Split(strings.TrimRight(memCol.String(), "\n"), "\n")

	// Combine columns line by line
	maxLines := max(len(cpuLines), len(memLines))
	for i := 0; i < maxLines; i++ {
		var cpuLine, memLine string
		if i < len(cpuLines) {
			cpuLine = cpuLines[i]
		}
		if i < len(memLines) {
			memLine = memLines[i]
		}

		// Apply colors based on line type
		var styledCpuLine, styledMemLine string
		if i == 0 {
			// Header lines - bold and gray
			styledCpuLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render(cpuLine)
			styledMemLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render(memLine)
		} else if strings.Contains(cpuLine, "No data") || strings.Contains(memLine, "No data") {
			// No data lines - gray
			styledCpuLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render(cpuLine)
			styledMemLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render(memLine)
		} else {
			// Data lines - bright white
			styledCpuLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6")).Render(cpuLine)
			styledMemLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6")).Render(memLine)
		}

		leftCol := lipgloss.NewStyle().Width(colWidth).Render(styledCpuLine)
		rightCol := lipgloss.NewStyle().Width(colWidth).Render(styledMemLine)
		content.WriteString(leftCol + "  " + rightCol + "\n")
	}

	contentStr := strings.TrimRight(content.String(), "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	return boxStyle.Render(contentStr)
}

type processInfo struct {
	name  string
	usage float64
}

// getTopProcesses reads /proc to get actual top processes by CPU time and memory
func getTopProcesses() ([]processInfo, []processInfo) {
	processes := []struct {
		name    string
		cpuTime uint64 // Total CPU time in jiffies (utime + stime)
		memRSS  uint64 // Memory in KB
	}{}

	// Read all /proc/[pid] directories
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, nil
	}

	for _, entry := range entries {
		// Skip if not a directory or not a numeric name (PID)
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}

		// Read process name from /proc/[pid]/comm
		commPath := filepath.Join("/proc", pid, "comm")
		commData, err := os.ReadFile(commPath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(commData))

		// Read CPU time from /proc/[pid]/stat
		statPath := filepath.Join("/proc", pid, "stat")
		statData, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		// Parse stat file: fields are space-separated
		// utime is field 14 (index 13), stime is field 15 (index 14)
		statFields := strings.Fields(string(statData))
		if len(statFields) < 15 {
			continue
		}

		utime, _ := strconv.ParseUint(statFields[13], 10, 64)
		stime, _ := strconv.ParseUint(statFields[14], 10, 64)
		cpuTime := utime + stime

		// Read memory from /proc/[pid]/status
		statusPath := filepath.Join("/proc", pid, "status")
		statusData, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}

		// Find VmRSS line (resident memory in KB)
		var memRSS uint64
		for _, line := range strings.Split(string(statusData), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					memRSS, _ = strconv.ParseUint(fields[1], 10, 64)
				}
				break
			}
		}

		processes = append(processes, struct {
			name    string
			cpuTime uint64
			memRSS  uint64
		}{name, cpuTime, memRSS})
	}

	// Sort by CPU time (descending)
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].cpuTime > processes[j].cpuTime
	})

	// Get top 3 CPU processes
	topCPU := []processInfo{}
	for i := 0; i < len(processes) && i < 3; i++ {
		// Convert CPU jiffies to percentage (simplified: just show relative value)
		topCPU = append(topCPU, processInfo{
			name:  processes[i].name,
			usage: float64(processes[i].cpuTime) / 100.0, // Simplified percentage
		})
	}

	// Sort by memory (descending)
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].memRSS > processes[j].memRSS
	})

	// Get top 3 memory processes
	topMem := []processInfo{}
	for i := 0; i < len(processes) && i < 3; i++ {
		topMem = append(topMem, processInfo{
			name:  processes[i].name,
			usage: float64(processes[i].memRSS) / 1024.0, // Convert KB to MB
		})
	}

	return topCPU, topMem
}

func generateSparkline(data []float64) string {
	if len(data) == 0 {
		return ""
	}

	// Sparkline characters from low to high
	sparks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Handle case where all values are the same
	if max == min {
		max = min + 1
	}

	result := make([]rune, len(data))
	for i, v := range data {
		// Normalize to 0-7 range for our 8 spark characters
		normalized := (v - min) / (max - min)
		index := int(normalized * 7)
		if index > 7 {
			index = 7
		}
		if index < 0 {
			index = 0
		}
		result[i] = sparks[index]
	}

	return string(result)
}

func (m model) renderCurrentMetrics() string {
	r := m.report

	// Calculate box width based on layout mode
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Side-by-side layout: calculate exact half width (same as serverInfo)
		// Total available: m.width - 4 (margins) - 1 (space between) = m.width - 5
		// Each box gets half: (m.width - 5) / 2
		// But Width() sets content width, so subtract borders (2) and padding (2)
		boxWidth = (m.width - 5) / 2 - 4
	} else {
		// Stack view: account for borders (2) and padding (2)
		boxWidth = boxWidth - 4
		maxStackWidth := 76 // 80 - 4 for borders and padding
		if boxWidth > maxStackWidth {
			boxWidth = maxStackWidth
		}
	}

	var content strings.Builder

	// Section title
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Accent).
		Render("📊 Current Metrics")
	content.WriteString(header + "\n")

	// Hostname and timestamp
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
	content.WriteString(
		lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render("Host: ") +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F3F4F6")).Render(r.Hostname) +
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

	contentStr := strings.TrimRight(content.String(), "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	return boxStyle.Render(contentStr)
}


func (m model) renderServerInfo() string {
	// Calculate box width based on layout mode
	boxWidth := m.width - 4
	if m.width >= 120 {
		// Side-by-side layout: calculate exact half width
		// Total available: m.width - 4 (margins) - 1 (space between) = m.width - 5
		// Each box gets half: (m.width - 5) / 2
		// But Width() sets content width, so subtract borders (2) and padding (2)
		boxWidth = (m.width - 5) / 2 - 4
	} else {
		// Stack view: account for borders (2) and padding (2)
		boxWidth = boxWidth - 4
		maxStackWidth := 76 // 80 - 4 for borders and padding
		if boxWidth > maxStackWidth {
			boxWidth = maxStackWidth
		}
	}

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

	// Current user
	currentUser := "unknown"
	if u, err := user.Current(); err == nil {
		currentUser = u.Username
	}
	content.WriteString(renderStatLine("Current User", currentUser))

	contentStr := strings.TrimRight(content.String(), "\n")

	// Add padding lines to match height with current metrics box
	contentStr += "\n\n"

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(boxWidth)

	return boxStyle.Render(contentStr)
}

func (m model) renderFooter() string {
	keys := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render(
		"[q] quit • [r] refresh • Updates every " + m.cfg.Agent.Interval.String(),
	)
	return keys
}

// Helper functions

func renderStatLine(label, value string) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#D1D5DB")).
		Width(16)
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F3F4F6")).
		Bold(true)
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
