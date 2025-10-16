package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/node-pulse/agent/cmd/themes"
	"github.com/node-pulse/agent/internal/installer"
)

// Screen represents different wizard screens
type Screen int

const (
	ScreenSplash Screen = iota // ASCII logo splash screen
	ScreenChecking             // Initial permission and installation checks
	ScreenWelcome
	ScreenEndpoint
	ScreenServerID
	ScreenInterval
	ScreenTimeout
	ScreenBuffer
	ScreenLogging
	ScreenReview
	ScreenInstalling
	ScreenSuccess
)

// initTUIModel represents the state of the TUI wizard
type initTUIModel struct {
	screen          Screen
	width           int
	height          int
	existing        *installer.ExistingInstall
	config          installer.ConfigOptions
	textInput       textinput.Model
	err             error
	installStep     int
	installSteps    []string
	checkingStep    int // 0=permissions, 1=existing, 2=done
	checkingSteps   []string
	quitting        bool
	permissionError error
}

type splashCompleteMsg struct{}

type checkStepMsg struct {
	step     int
	existing *installer.ExistingInstall
	err      error
}

type installStepMsg struct {
	step int
	err  error
}

type installCompleteMsg struct {
	config installer.ConfigOptions
}

// newInitTUIModel creates a new TUI model
func newInitTUIModel() initTUIModel {
	ti := textinput.New()
	ti.Placeholder = "https://ingest.nodepulse.sh"
	ti.Focus()
	ti.Width = 60

	return initTUIModel{
		screen:    ScreenSplash,
		config:    installer.DefaultConfigOptions(),
		textInput: ti,
		checkingSteps: []string{
			"Checking permissions",
			"Detecting existing installation",
		},
		installSteps: []string{
			"Creating directories",
			"Persisting server ID",
			"Writing configuration file",
			"Setting permissions",
			"Validating installation",
		},
	}
}

func (m initTUIModel) Init() tea.Cmd {
	// Start with splash screen for 1500ms
	return func() tea.Msg {
		time.Sleep(1500 * time.Millisecond)
		return splashCompleteMsg{}
	}
}

func (m initTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			return m.handleEnter()

		default:
			// Handle text input
			if m.screen == ScreenEndpoint || m.screen == ScreenServerID ||
				m.screen == ScreenInterval || m.screen == ScreenTimeout ||
				m.screen == ScreenBuffer || m.screen == ScreenLogging {
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case splashCompleteMsg:
		// Splash screen complete, move to checking
		m.screen = ScreenChecking
		return m, m.runCheckStep(0)

	case checkStepMsg:
		if msg.err != nil {
			m.permissionError = msg.err
			m.err = msg.err
			m.quitting = true
			return m, tea.Quit
		}
		m.checkingStep = msg.step
		if msg.existing != nil {
			m.existing = msg.existing
		}
		if msg.step < len(m.checkingSteps) {
			return m, m.runCheckStep(msg.step)
		}
		// Checking complete, move to welcome screen
		m.screen = ScreenWelcome
		return m, textinput.Blink

	case installStepMsg:
		if msg.err != nil {
			m.err = msg.err
			m.quitting = true
			return m, tea.Quit
		}
		m.installStep = msg.step
		if msg.step < len(m.installSteps) {
			return m, m.runInstallStep(msg.step)
		}
		// Installation complete
		return m, func() tea.Msg {
			return installCompleteMsg{config: m.config}
		}

	case installCompleteMsg:
		m.screen = ScreenSuccess
		return m, nil
	}

	return m, nil
}

func (m initTUIModel) View() string {
	if m.quitting {
		if m.err != nil {
			return lipgloss.NewStyle().
				Foreground(themes.Current.Error).
				Render(fmt.Sprintf("✗ Error: %v\n", m.err))
		}
		return ""
	}

	switch m.screen {
	case ScreenSplash:
		return m.viewSplash()
	case ScreenChecking:
		return m.viewChecking()
	case ScreenWelcome:
		return m.viewWelcome()
	case ScreenEndpoint:
		return m.viewEndpoint()
	case ScreenServerID:
		return m.viewServerID()
	case ScreenInterval:
		return m.viewInterval()
	case ScreenTimeout:
		return m.viewTimeout()
	case ScreenBuffer:
		return m.viewBuffer()
	case ScreenLogging:
		return m.viewLogging()
	case ScreenReview:
		return m.viewReview()
	case ScreenInstalling:
		return m.viewInstalling()
	case ScreenSuccess:
		return m.viewSuccess()
	default:
		return ""
	}
}

func (m initTUIModel) viewSplash() string {
	logo := `
███╗   ██╗ ██████╗ ██████╗ ███████╗
████╗  ██║██╔═══██╗██╔══██╗██╔════╝
██╔██╗ ██║██║   ██║██║  ██║█████╗
██║╚██╗██║██║   ██║██║  ██║██╔══╝
██║ ╚████║╚██████╔╝██████╔╝███████╗
╚═╝  ╚═══╝ ╚═════╝ ╚═════╝ ╚══════╝

██████╗ ██╗   ██╗██╗     ███████╗███████╗
██╔══██╗██║   ██║██║     ██╔════╝██╔════╝
██████╔╝██║   ██║██║     ███████╗█████╗
██╔═══╝ ██║   ██║██║     ╚════██║██╔══╝
██║     ╚██████╔╝███████╗███████║███████╗
╚═╝      ╚═════╝ ╚══════╝╚══════╝╚══════╝
`

	logoStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		Align(lipgloss.Center)

	taglineStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Align(lipgloss.Center).
		Italic(true).
		MarginTop(2)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		logoStyle.Render(logo)+"\n"+taglineStyle.Render("Real-time server monitoring agent"),
	)
}

func (m initTUIModel) viewChecking() string {
	title := `
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║      ⚡ NodePulse Agent Initialization                     ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
`

	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")

	for i, step := range m.checkingSteps {
		var line string
		if i < m.checkingStep {
			// Completed
			checkStyle := lipgloss.NewStyle().Foreground(themes.Current.Success)
			textStyle := lipgloss.NewStyle().Foreground(themes.Current.TextPrimary)
			line = checkStyle.Render("✓ ") + textStyle.Render(step)
		} else if i == m.checkingStep {
			// In progress
			spinStyle := lipgloss.NewStyle().Foreground(themes.Current.Accent)
			textStyle := lipgloss.NewStyle().Foreground(themes.Current.TextPrimary)
			line = spinStyle.Render("⟳ ") + textStyle.Render(step+"...")
		} else {
			// Pending
			pendingStyle := lipgloss.NewStyle().Foreground(themes.Current.TextPrimary).Faint(true)
			line = pendingStyle.Render("○ " + step)
		}
		b.WriteString(contentStyle.Render(line) + "\n")
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewWelcome() string {
	title := `
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║      ⚡ NodePulse Agent Initialization                    ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
`

	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")

	b.WriteString(contentStyle.Render(textStyle.Render("This wizard will help you set up the NodePulse agent.")))
	b.WriteString("\n\n")
	b.WriteString(contentStyle.Render(textStyle.Render("It will:")))
	b.WriteString("\n\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  • Create necessary directories")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  • Generate or use a server ID")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  • Create configuration file")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  • Set proper permissions")))
	b.WriteString("\n\n")

	if m.existing != nil && (m.existing.HasConfig || m.existing.HasServerID) {
		warningStyle := lipgloss.NewStyle().
			Foreground(themes.Current.Warning).
			Bold(true)

		b.WriteString(contentStyle.Render(warningStyle.Render("⚠ Existing installation detected")))
		b.WriteString("\n\n")

		if m.existing.HasConfig {
			b.WriteString(contentStyle.Render(textStyle.Render(fmt.Sprintf("  Config: %s", m.existing.ConfigPath))))
			b.WriteString("\n")
		}
		if m.existing.HasServerID {
			b.WriteString(contentStyle.Render(textStyle.Render(fmt.Sprintf("  Server ID: %s", strings.TrimSpace(m.existing.ServerID)))))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	b.WriteString(contentStyle.Render(helpStyle.Render("Press Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewEndpoint() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("📡 Endpoint Configuration"))
	b.WriteString("\n\n")

	if m.existing != nil && m.existing.Endpoint != "" {
		b.WriteString(contentStyle.Render(textStyle.Render("Edit the endpoint URL or press Enter to keep it:")))
	} else {
		b.WriteString(contentStyle.Render(textStyle.Render("Enter the metrics endpoint URL for your control server:")))
	}
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(m.textInput.View()))
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(themes.Current.Error)
		b.WriteString(contentStyle.Render(errorStyle.Render(fmt.Sprintf("❌ %v", m.err))))
		b.WriteString("\n\n")
	}

	b.WriteString(contentStyle.Render(textStyle.Render("Example: https://api.nodepulse.io/metrics")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewServerID() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("🔑 Server ID Configuration"))
	b.WriteString("\n\n")

	if m.existing != nil && m.existing.HasServerID {
		b.WriteString(contentStyle.Render(textStyle.Render("Edit the server ID or press Enter to keep it:")))
	} else {
		b.WriteString(contentStyle.Render(textStyle.Render("Enter a server ID or leave empty to auto-generate UUID:")))
	}

	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(m.textInput.View()))
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(themes.Current.Error)
		b.WriteString(contentStyle.Render(errorStyle.Render(fmt.Sprintf("❌ %v", m.err))))
		b.WriteString("\n\n")
	}

	b.WriteString(contentStyle.Render(textStyle.Render("Examples: prod-web-01, my-server, database-primary")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("Format: letters, numbers, and dashes only")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewInterval() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("⏱️  Metrics Collection Interval"))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(textStyle.Render("How often should metrics be collected?")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(m.textInput.View()))
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(themes.Current.Error)
		b.WriteString(contentStyle.Render(errorStyle.Render(fmt.Sprintf("❌ %v", m.err))))
		b.WriteString("\n\n")
	}

	b.WriteString(contentStyle.Render(textStyle.Render("Allowed values: 5s, 10s, 30s, 1m")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewTimeout() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("⏰ HTTP Request Timeout"))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(textStyle.Render("Maximum time to wait for server response:")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(m.textInput.View()))
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(themes.Current.Error)
		b.WriteString(contentStyle.Render(errorStyle.Render(fmt.Sprintf("❌ %v", m.err))))
		b.WriteString("\n\n")
	}

	b.WriteString(contentStyle.Render(textStyle.Render("Example: 3s (3 seconds)")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewBuffer() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("💾 Local Buffer Configuration"))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(textStyle.Render("Enable local buffering of failed reports?")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("(Failed reports will be stored locally and retried later)")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(m.textInput.View()))
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(themes.Current.Error)
		b.WriteString(contentStyle.Render(errorStyle.Render(fmt.Sprintf("❌ %v", m.err))))
		b.WriteString("\n\n")
	}

	b.WriteString(contentStyle.Render(textStyle.Render("Recommended: yes")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewLogging() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("📝 Logging Configuration"))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(textStyle.Render("Set the logging level:")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(m.textInput.View()))
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(themes.Current.Error)
		b.WriteString(contentStyle.Render(errorStyle.Render(fmt.Sprintf("❌ %v", m.err))))
		b.WriteString("\n\n")
	}

	b.WriteString(contentStyle.Render(textStyle.Render("debug: Verbose diagnostic information")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("info:  General informational messages (recommended)")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("warn:  Warning messages")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("error: Error messages only")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Enter to continue • Esc to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewReview() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true).
		Width(18)

	valueStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(themes.Current.Accent).
		Padding(2, 3).
		MarginTop(0).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("📋 Review Configuration"))
	b.WriteString("\n")

	// Configuration summary
	var summary strings.Builder
	summary.WriteString(labelStyle.Render("Endpoint:") + " " + valueStyle.Render(m.config.Endpoint) + "\n")
	summary.WriteString(labelStyle.Render("Server ID:") + " " + valueStyle.Render(m.config.ServerID) + "\n")
	summary.WriteString(labelStyle.Render("Interval:") + " " + valueStyle.Render(m.config.Interval) + "\n")
	summary.WriteString(labelStyle.Render("Timeout:") + " " + valueStyle.Render(m.config.Timeout) + "\n")
	bufferStatus := "Disabled"
	if m.config.BufferEnabled {
		bufferStatus = fmt.Sprintf("Enabled (%dh retention)", m.config.BufferRetentionHours)
	}
	summary.WriteString(labelStyle.Render("Buffer:") + " " + valueStyle.Render(bufferStatus) + "\n")
	summary.WriteString(labelStyle.Render("Logging:") + " " + valueStyle.Render(fmt.Sprintf("%s → %s", m.config.LogLevel, m.config.LogOutput)) + "\n")
	summary.WriteString(labelStyle.Render("Config Path:") + " " + valueStyle.Render("/etc/node-pulse/nodepulse.yml"))

	b.WriteString(contentStyle.Render(boxStyle.Render(summary.String())))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Press Enter to install • Esc to cancel")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewInstalling() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Accent).
		Bold(true).
		MarginBottom(1)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("⚙️  Installing..."))
	b.WriteString("\n\n")

	for i, step := range m.installSteps {
		var line string
		if i < m.installStep {
			// Completed
			checkStyle := lipgloss.NewStyle().Foreground(themes.Current.Success)
			textStyle := lipgloss.NewStyle().Foreground(themes.Current.TextPrimary)
			line = checkStyle.Render("✓ ") + textStyle.Render(step)
		} else if i == m.installStep {
			// In progress
			spinStyle := lipgloss.NewStyle().Foreground(themes.Current.Accent)
			textStyle := lipgloss.NewStyle().Foreground(themes.Current.TextPrimary)
			line = spinStyle.Render("⟳ ") + textStyle.Render(step+"...")
		} else {
			// Pending
			pendingStyle := lipgloss.NewStyle().Foreground(themes.Current.TextPrimary).Faint(true)
			line = pendingStyle.Render("○ " + step)
		}
		b.WriteString(contentStyle.Render(line) + "\n")
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) viewSuccess() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(themes.Current.Success).
		Bold(true).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary)

	labelStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Bold(true)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(themes.Current.Success).
		Padding(2, 3).
		MarginTop(0).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(themes.Current.TextPrimary).
		Faint(true)

	contentStyle := lipgloss.NewStyle().
		Padding(0, 4)

	var b strings.Builder

	b.WriteString(titleStyle.Render("✓ Node Pulse agent initialized successfully!"))
	b.WriteString("\n")

	// Summary box
	var summary strings.Builder
	summary.WriteString(labelStyle.Render("Server ID:") + " " + valueStyle.Render(m.config.ServerID) + "\n")
	summary.WriteString(labelStyle.Render("Config:") + " " + valueStyle.Render("/etc/node-pulse/nodepulse.yml"))

	b.WriteString(contentStyle.Render(boxStyle.Render(summary.String())))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(textStyle.Render("Next steps:")))
	b.WriteString("\n\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  1. Start the agent:    pulse agent")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  2. Watch live metrics: pulse watch")))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render(textStyle.Render("  3. Install service:    sudo pulse service install")))
	b.WriteString("\n\n")

	b.WriteString(contentStyle.Render(helpStyle.Render("Press any key to exit")))

	return lipgloss.NewStyle().Padding(2, 4).Render(b.String())
}

func (m initTUIModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenWelcome:
		// Move to endpoint screen
		m.screen = ScreenEndpoint
		m.textInput.Placeholder = "https://api.nodepulse.io/metrics"
		// Pre-populate with existing endpoint if available
		if m.existing != nil && m.existing.Endpoint != "" {
			m.textInput.SetValue(m.existing.Endpoint)
		} else {
			m.textInput.SetValue("")
		}
		m.textInput.Focus()
		m.err = nil
		return m, textinput.Blink

	case ScreenEndpoint:
		// Validate endpoint
		endpoint := strings.TrimSpace(m.textInput.Value())
		if endpoint == "" {
			m.err = fmt.Errorf("endpoint is required")
			return m, nil
		}
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			m.err = fmt.Errorf("endpoint must start with http:// or https://")
			return m, nil
		}

		m.config.Endpoint = endpoint
		m.err = nil

		// Move to server ID screen
		m.screen = ScreenServerID
		if m.existing != nil && m.existing.HasServerID {
			m.textInput.Placeholder = "Leave empty to auto-generate UUID"
			// Pre-populate with existing server ID
			m.textInput.SetValue(strings.TrimSpace(m.existing.ServerID))
		} else {
			m.textInput.Placeholder = "Leave empty to auto-generate UUID"
			m.textInput.SetValue("")
		}
		m.textInput.Focus()
		return m, textinput.Blink

	case ScreenServerID:
		// Handle server ID
		serverID := strings.TrimSpace(m.textInput.Value())

		if serverID == "" {
			// Use existing or generate
			if m.existing != nil && m.existing.HasServerID {
				m.config.ServerID = strings.TrimSpace(m.existing.ServerID)
			} else {
				// Will generate UUID
				uuid, err := installer.HandleServerID("")
				if err != nil {
					m.err = err
					return m, nil
				}
				m.config.ServerID = uuid
			}
		} else {
			// Validate custom server ID
			if err := installer.ValidateServerID(serverID); err != nil {
				m.err = err
				return m, nil
			}
			m.config.ServerID = serverID
		}

		m.err = nil

		// Move to interval screen
		m.screen = ScreenInterval
		m.textInput.SetValue(m.config.Interval)
		m.textInput.Placeholder = "5s, 10s, 30s, 1m"
		m.textInput.Focus()
		return m, textinput.Blink

	case ScreenInterval:
		// Validate interval
		interval := strings.TrimSpace(m.textInput.Value())
		validIntervals := map[string]bool{"5s": true, "10s": true, "30s": true, "1m": true}
		if !validIntervals[interval] {
			m.err = fmt.Errorf("interval must be one of: 5s, 10s, 30s, 1m")
			return m, nil
		}

		m.config.Interval = interval
		m.err = nil

		// Move to timeout screen
		m.screen = ScreenTimeout
		m.textInput.SetValue(m.config.Timeout)
		m.textInput.Placeholder = "3s"
		m.textInput.Focus()
		return m, textinput.Blink

	case ScreenTimeout:
		// Validate timeout (just check it's not empty and ends with 's')
		timeout := strings.TrimSpace(m.textInput.Value())
		if timeout == "" || !strings.HasSuffix(timeout, "s") {
			m.err = fmt.Errorf("timeout must be a duration like '3s', '5s', etc.")
			return m, nil
		}

		m.config.Timeout = timeout
		m.err = nil

		// Move to buffer screen
		m.screen = ScreenBuffer
		if m.config.BufferEnabled {
			m.textInput.SetValue("yes")
		} else {
			m.textInput.SetValue("no")
		}
		m.textInput.Placeholder = "yes/no"
		m.textInput.Focus()
		return m, textinput.Blink

	case ScreenBuffer:
		// Parse buffer settings
		input := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
		if input != "yes" && input != "no" {
			m.err = fmt.Errorf("enter 'yes' or 'no'")
			return m, nil
		}

		m.config.BufferEnabled = (input == "yes")
		m.err = nil

		// Move to logging screen
		m.screen = ScreenLogging
		m.textInput.SetValue(m.config.LogLevel)
		m.textInput.Placeholder = "debug, info, warn, error"
		m.textInput.Focus()
		return m, textinput.Blink

	case ScreenLogging:
		// Validate log level
		logLevel := strings.ToLower(strings.TrimSpace(m.textInput.Value()))
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !validLevels[logLevel] {
			m.err = fmt.Errorf("log level must be one of: debug, info, warn, error")
			return m, nil
		}

		m.config.LogLevel = logLevel
		m.err = nil

		// Move to review screen
		m.screen = ScreenReview
		return m, nil

	case ScreenReview:
		// Start installation
		m.screen = ScreenInstalling
		m.installStep = 0
		return m, m.runInstallStep(0)

	case ScreenSuccess:
		m.quitting = true
		return m, tea.Quit

	default:
		return m, nil
	}
}

func (m initTUIModel) runInstallStep(step int) tea.Cmd {
	return func() tea.Msg {
		var err error

		switch step {
		case 0: // Create directories
			err = installer.CreateDirectories()
		case 1: // Persist server ID
			err = installer.PersistServerID(m.config.ServerID)
		case 2: // Write config file
			err = installer.WriteConfigFile(m.config)
		case 3: // Fix permissions
			err = installer.FixPermissions()
		case 4: // Validate installation
			err = installer.ValidateInstallation()
		}

		if err != nil {
			return installStepMsg{step: step, err: err}
		}

		return installStepMsg{step: step + 1, err: nil}
	}
}

func (m initTUIModel) runCheckStep(step int) tea.Cmd {
	return func() tea.Msg {
		var err error
		var existing *installer.ExistingInstall

		switch step {
		case 0: // Check permissions
			err = installer.CheckPermissions()
		case 1: // Detect existing installation
			existing, err = installer.DetectExisting()
			if err != nil {
				return checkStepMsg{step: step, err: fmt.Errorf("failed to detect existing installation: %w", err)}
			}
		}

		if err != nil {
			return checkStepMsg{step: step, err: err}
		}

		return checkStepMsg{step: step + 1, existing: existing, err: nil}
	}
}
