package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/node-pulse/agent/internal/installer"
)

// Screen represents different wizard screens
type Screen int

const (
	ScreenWelcome Screen = iota
	ScreenEndpoint
	ScreenServerID
	ScreenReview
	ScreenInstalling
	ScreenSuccess
)

// initTUIModel represents the state of the TUI wizard
type initTUIModel struct {
	screen         Screen
	width          int
	height         int
	existing       *installer.ExistingInstall
	endpoint       string
	serverID       string
	useExistingID  bool
	textInput      textinput.Model
	err            error
	installStep    int
	installSteps   []string
	quitting       bool
}

type installStepMsg struct {
	step int
	err  error
}

type installCompleteMsg struct {
	serverID string
}

// newInitTUIModel creates a new TUI model
func newInitTUIModel(existing *installer.ExistingInstall) initTUIModel {
	ti := textinput.New()
	ti.Placeholder = "https://api.nodepulse.io/metrics"
	ti.Focus()
	ti.Width = 60

	return initTUIModel{
		screen:    ScreenWelcome,
		existing:  existing,
		textInput: ti,
		installSteps: []string{
			"Checking permissions",
			"Creating directories",
			"Persisting server ID",
			"Writing configuration file",
			"Setting permissions",
			"Validating installation",
		},
	}
}

func (m initTUIModel) Init() tea.Cmd {
	return textinput.Blink
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
			if m.screen == ScreenEndpoint || m.screen == ScreenServerID {
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

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
			return installCompleteMsg{serverID: m.serverID}
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
				Foreground(errorColor).
				Render(fmt.Sprintf("✗ Error: %v\n", m.err))
		}
		return ""
	}

	switch m.screen {
	case ScreenWelcome:
		return m.viewWelcome()
	case ScreenEndpoint:
		return m.viewEndpoint()
	case ScreenServerID:
		return m.viewServerID()
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

func (m initTUIModel) viewWelcome() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		MarginBottom(1)

	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ NodePulse Agent Initialization"))
	b.WriteString("\n\n")

	b.WriteString(textStyle.Render("This wizard will help you set up the NodePulse agent."))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("It will:"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  • Create necessary directories"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  • Generate or use a server ID"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  • Create configuration file"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  • Set proper permissions"))
	b.WriteString("\n\n")

	if m.existing.HasConfig || m.existing.HasServerID {
		warningStyle := lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

		b.WriteString(warningStyle.Render("⚠ Existing installation detected"))
		b.WriteString("\n\n")

		if m.existing.HasConfig {
			b.WriteString(textStyle.Render(fmt.Sprintf("  Config: %s", m.existing.ConfigPath)))
			b.WriteString("\n")
		}
		if m.existing.HasServerID {
			b.WriteString(textStyle.Render(fmt.Sprintf("  Server ID: %s", strings.TrimSpace(m.existing.ServerID))))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	helpStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Faint(true)

	b.WriteString(helpStyle.Render("Press Enter to continue • Esc to exit"))

	return b.String()
}

func (m initTUIModel) viewEndpoint() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Faint(true).
		MarginTop(1)

	var b strings.Builder

	b.WriteString(titleStyle.Render("📡 Endpoint Configuration"))
	b.WriteString("\n\n")

	b.WriteString(textStyle.Render("Enter the metrics endpoint URL for your control server:"))
	b.WriteString("\n\n")

	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(errorColor)
		b.WriteString(errorStyle.Render(fmt.Sprintf("❌ %v", m.err)))
		b.WriteString("\n\n")
	}

	b.WriteString(textStyle.Render("Example: https://api.nodepulse.io/metrics"))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Enter to continue • Esc to exit"))

	return b.String()
}

func (m initTUIModel) viewServerID() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Faint(true).
		MarginTop(1)

	var b strings.Builder

	b.WriteString(titleStyle.Render("🔑 Server ID Configuration"))
	b.WriteString("\n\n")

	if m.existing.HasServerID {
		b.WriteString(textStyle.Render(fmt.Sprintf("Found existing server ID: %s", strings.TrimSpace(m.existing.ServerID))))
		b.WriteString("\n\n")
		b.WriteString(textStyle.Render("Enter a new server ID or leave empty to keep existing:"))
	} else {
		b.WriteString(textStyle.Render("Enter a server ID or leave empty to auto-generate UUID:"))
	}

	b.WriteString("\n\n")

	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(errorColor)
		b.WriteString(errorStyle.Render(fmt.Sprintf("❌ %v", m.err)))
		b.WriteString("\n\n")
	}

	b.WriteString(textStyle.Render("Examples: prod-web-01, my-server, database-primary"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("Format: letters, numbers, and dashes only"))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Enter to continue • Esc to exit"))

	return b.String()
}

func (m initTUIModel) viewReview() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(18)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Faint(true)

	var b strings.Builder

	b.WriteString(titleStyle.Render("📋 Review Configuration"))
	b.WriteString("\n\n")

	// Configuration summary
	var summary strings.Builder
	summary.WriteString(labelStyle.Render("Endpoint:") + " " + valueStyle.Render(m.endpoint) + "\n")
	summary.WriteString(labelStyle.Render("Server ID:") + " " + valueStyle.Render(m.serverID) + "\n")
	summary.WriteString(labelStyle.Render("Interval:") + " " + valueStyle.Render("5s") + "\n")
	summary.WriteString(labelStyle.Render("Timeout:") + " " + valueStyle.Render("3s") + "\n")
	summary.WriteString(labelStyle.Render("Buffer:") + " " + valueStyle.Render("Enabled (48h retention)") + "\n")
	summary.WriteString(labelStyle.Render("Config Path:") + " " + valueStyle.Render("/etc/node-pulse/nodepulse.yml"))

	b.WriteString(boxStyle.Render(summary.String()))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Press Enter to install • Esc to cancel"))

	return b.String()
}

func (m initTUIModel) viewInstalling() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		MarginBottom(1)

	var b strings.Builder

	b.WriteString(titleStyle.Render("⚙️  Installing..."))
	b.WriteString("\n\n")

	for i, step := range m.installSteps {
		if i < m.installStep {
			// Completed
			checkStyle := lipgloss.NewStyle().Foreground(successColor)
			b.WriteString(checkStyle.Render("✓ ") + step + "\n")
		} else if i == m.installStep {
			// In progress
			spinStyle := lipgloss.NewStyle().Foreground(accentColor)
			b.WriteString(spinStyle.Render("⟳ ") + step + "...\n")
		} else {
			// Pending
			pendingStyle := lipgloss.NewStyle().Foreground(mutedColor)
			b.WriteString(pendingStyle.Render("○ ") + step + "\n")
		}
	}

	return b.String()
}

func (m initTUIModel) viewSuccess() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(successColor).
		MarginBottom(1)

	textStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB")).
		Bold(true)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(successColor).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Faint(true)

	var b strings.Builder

	b.WriteString(titleStyle.Render("✓ NodePulse agent initialized successfully!"))
	b.WriteString("\n\n")

	// Summary box
	var summary strings.Builder
	summary.WriteString(labelStyle.Render("Server ID:") + " " + valueStyle.Render(m.serverID) + "\n")
	summary.WriteString(labelStyle.Render("Config:") + " " + valueStyle.Render("/etc/node-pulse/nodepulse.yml"))

	b.WriteString(boxStyle.Render(summary.String()))
	b.WriteString("\n\n")

	b.WriteString(textStyle.Render("Next steps:"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  1. Start the agent:    pulse agent"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  2. View live metrics:  pulse view"))
	b.WriteString("\n")
	b.WriteString(textStyle.Render("  3. Install service:    sudo pulse service install"))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Press any key to exit"))

	return b.String()
}

func (m initTUIModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenWelcome:
		// Move to endpoint screen
		m.screen = ScreenEndpoint
		m.textInput.Placeholder = "https://api.nodepulse.io/metrics"
		m.textInput.SetValue("")
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

		m.endpoint = endpoint
		m.err = nil

		// Move to server ID screen
		m.screen = ScreenServerID
		m.textInput.Placeholder = ""
		if m.existing.HasServerID {
			m.textInput.Placeholder = "Leave empty to keep: " + strings.TrimSpace(m.existing.ServerID)
		} else {
			m.textInput.Placeholder = "Leave empty to auto-generate UUID"
		}
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case ScreenServerID:
		// Handle server ID
		serverID := strings.TrimSpace(m.textInput.Value())

		if serverID == "" {
			// Use existing or generate
			if m.existing.HasServerID {
				m.serverID = strings.TrimSpace(m.existing.ServerID)
			} else {
				// Will generate UUID
				uuid, err := installer.HandleServerID("")
				if err != nil {
					m.err = err
					return m, nil
				}
				m.serverID = uuid
			}
		} else {
			// Validate custom server ID
			if err := installer.ValidateServerID(serverID); err != nil {
				m.err = err
				return m, nil
			}
			m.serverID = serverID
		}

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
		case 0: // Check permissions
			err = installer.CheckPermissions()
		case 1: // Create directories
			err = installer.CreateDirectories()
		case 2: // Persist server ID
			err = installer.PersistServerID(m.serverID)
		case 3: // Write config file
			err = installer.WriteConfigFile(m.endpoint, m.serverID)
		case 4: // Fix permissions
			err = installer.FixPermissions()
		case 5: // Validate installation
			err = installer.ValidateInstallation()
		}

		if err != nil {
			return installStepMsg{step: step, err: err}
		}

		return installStepMsg{step: step + 1, err: nil}
	}
}
