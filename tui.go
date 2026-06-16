package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	target    TargetSpec
	state     StoredState
	probe     ProbeResult
	result    DownloadResult
	logs      []string
	pending   int
	phase     string
	frame     int
	width     int
	height    int
	lastError string
}

type loadStateMsg struct {
	State StoredState
	Err   error
}

type probeMsg struct {
	Result ProbeResult
	Err    error
}

type downloadMsg struct {
	Result DownloadResult
	Err    error
}

type tickMsg struct{}

var spinnerFrames = []string{"|", "/", "-", `\`}

func newModel(target TargetSpec) model {
	return model{
		target:  target,
		probe:   ProbeResult{Target: target},
		logs:    []string{"Starting codex-unpacker."},
		pending: 2,
		phase:   "checking",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		loadStateCmd(),
		probeCmd(m.target),
		tickCmd(),
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
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			if m.pending == 0 {
				m.beginWork(2, "checking")
				m.appendLog("Refreshing saved state and upstream metadata.")
				return m, tea.Batch(loadStateCmd(), probeCmd(m.target), tickCmd())
			}
		case "d":
			if m.pending == 0 {
				m.beginWork(1, "downloading")
				m.appendLog("Downloading the latest Codex package to Downloads.")
				return m, tea.Batch(downloadCmd(m.target, ""), tickCmd())
			}
		}
		return m, nil
	case tickMsg:
		if m.pending > 0 {
			m.frame = (m.frame + 1) % len(spinnerFrames)
			return m, tickCmd()
		}
		return m, nil
	case loadStateMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.appendLog("State load error: " + msg.Err.Error())
		} else {
			m.state = msg.State
			saved := stateForTarget(m.state, m.target)
			if saved.Package.Version == "" {
				m.appendLog("No saved package yet for " + targetLabel(m.target) + ".")
			} else {
				m.appendLog("Loaded saved package " + saved.Package.Version + " for " + targetLabel(m.target) + ".")
			}
		}
		m.finishWork()
		return m, m.nextTick()
	case probeMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.appendLog("Probe error: " + msg.Err.Error())
		} else {
			m.probe = msg.Result
			m.probe.Target = m.target
			if msg.Result.Source.Version != "" {
				if msg.Result.WouldUpdate {
					m.appendLog("Latest Codex version found for " + targetLabel(m.target) + ": " + msg.Result.Source.Version + ".")
				} else {
					m.appendLog("Saved copy already matches the latest Codex version for " + targetLabel(m.target) + ".")
				}
			}
		}
		m.finishWork()
		return m, m.nextTick()
	case downloadMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			m.appendLog("Download error: " + msg.Err.Error())
		} else {
			m.result = msg.Result
			now := time.Now().UTC().Format(time.RFC3339)
			if m.state.Targets == nil {
				m.state.Targets = map[string]StoredTargetState{}
			}
			entry := StoredTargetState{
				UpdatedAt: now,
				Package:   msg.Result.Package,
				Source:    msg.Result.Source,
			}
			m.state.SchemaVersion = 2
			m.state.UpdatedAt = now
			m.state.Targets[targetKey(m.target)] = entry
			m.probe.State = entry
			m.probe.Source = msg.Result.Source
			m.probe.DefaultDestination = msg.Result.Destination
			m.probe.WouldUpdate = false
			m.probe.Target = m.target
			m.appendLog("Saved " + msg.Result.Package.Version + " for " + targetLabel(m.target) + " to " + msg.Result.Destination + ".")
		}
		m.finishWork()
		return m, m.nextTick()
	}
	return m, nil
}

func (m model) View() string {
	title := titleStyle.Render("codex-unpacker v" + appVersion)
	if m.pending > 0 {
		title += " " + spinnerStyle.Render(spinnerFrames[m.frame%len(spinnerFrames)])
	}

	subtitle := subtitleStyle.Render("Downloads the latest Codex package to your Downloads folder.")

	saved := infoCard("Saved", savedSummary(stateForTarget(m.state, m.target)))
	latest := infoCard("Latest", latestSummary(m.probe))
	target := infoCard("Target", targetSummary(m.probe))
	summary := summaryRow(m.width, saved, latest, target)

	status := statusLine(m)
	activity := logPanel(m.width, strings.Join(m.logs, "\n"))
	footer := footerStyle.Render("[r] refresh   [d] download   [q] quit")
	if m.lastError != "" {
		footer = errorStyle.Render(m.lastError)
	}

	return outerStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		summary,
		"",
		status,
		"",
		activity,
		"",
		footer,
	))
}

func loadStateCmd() tea.Cmd {
	return func() tea.Msg {
		state, err := LoadState()
		return loadStateMsg{State: state, Err: err}
	}
}

func probeCmd(target TargetSpec) tea.Cmd {
	return func() tea.Msg {
		result, err := ProbeLatest(target)
		return probeMsg{Result: result, Err: err}
	}
}

func downloadCmd(target TargetSpec, output string) tea.Cmd {
	return func() tea.Msg {
		result, err := DownloadLatest(target, output)
		return downloadMsg{Result: result, Err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *model) beginWork(pending int, phase string) {
	m.pending = pending
	m.phase = phase
	m.lastError = ""
}

func (m *model) finishWork() {
	if m.pending > 0 {
		m.pending--
	}
	if m.pending <= 0 {
		m.pending = 0
		m.phase = "idle"
	}
}

func (m model) nextTick() tea.Cmd {
	if m.pending > 0 {
		return tickCmd()
	}
	return nil
}

func (m *model) appendLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	line := fmt.Sprintf("%s  %s", timestamp, message)
	m.logs = append([]string{line}, m.logs...)
	if len(m.logs) > 8 {
		m.logs = m.logs[:8]
	}
}

func statusLine(m model) string {
	switch {
	case m.lastError != "":
		return statusErrorStyle.Render("issue") + " " + statusMutedStyle.Render("check the message below")
	case m.pending > 0:
		return statusBusyStyle.Render(strings.ToUpper(m.phase)) + " " + statusMutedStyle.Render("working")
	default:
		return statusReadyStyle.Render("READY") + " " + statusMutedStyle.Render("download is idle")
	}
}

func savedSummary(state StoredTargetState) string {
	if state.Package.Version == "" {
		return "No saved package yet."
	}

	lines := []string{
		state.Package.Version,
		"SHA " + shortHash(state.Package.SHA256),
	}
	if state.Package.Target.Platform != "" {
		lines = append([]string{targetLabel(state.Package.Target)}, lines...)
	}
	if state.UpdatedAt != "" {
		lines = append(lines, formatTimestamp(state.UpdatedAt))
	}
	return strings.Join(lines, "\n")
}

func latestSummary(probe ProbeResult) string {
	if probe.Source.Version == "" {
		return "Waiting for metadata."
	}

	lines := []string{
		probe.Source.Version,
		packageKindLabel(probe.Source.PackageKind),
		probe.Source.SourceKind,
	}
	if probe.Source.Size > 0 {
		lines = append(lines, formatBytes(probe.Source.Size))
	}
	if probe.Source.ExpectedSHA256 != "" {
		lines = append(lines, shortHash(probe.Source.ExpectedSHA256))
	}
	return strings.Join(lines, "\n")
}

func targetSummary(probe ProbeResult) string {
	lines := []string{targetLabel(probe.Target)}
	if probe.DefaultDestination == "" {
		lines = append(lines, "Downloads folder")
	} else {
		lines = append(lines, splitPathForCard(probe.DefaultDestination))
	}
	return strings.Join(lines, "\n")
}

func infoCard(title, body string) string {
	return cardStyle.Width(cardWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			cardTitleStyle.Render(title),
			cardBodyStyle.Render(body),
		),
	)
}

func summaryRow(width int, cards ...string) string {
	if len(cards) == 0 {
		return ""
	}
	if width < 96 {
		return lipgloss.JoinVertical(lipgloss.Left, cards...)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func logPanel(width int, body string) string {
	if strings.TrimSpace(body) == "" {
		body = "Waiting for activity."
	}

	panelWidth := clamp(width-4, 52, 96)

	return panelStyle.Width(panelWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			panelHeaderStyle.Render("Activity"),
			panelBodyStyle.Render(body),
		),
	)
}

func splitPathForCard(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	return truncateMiddle(dir, 22) + "\n" + truncateMiddle(base, 24)
}

func formatTimestamp(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Local().Format("2006-01-02 15:04")
}

func shortHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "0 B"
	}

	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	value := float64(size) / float64(div)
	suffixes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", value, suffixes[exp])
}

func truncateMiddle(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 3 {
		return string(runes[:max])
	}

	left := (max - 1) / 2
	right := max - 1 - left
	return string(runes[:left]) + "..." + string(runes[len(runes)-right:])
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

var (
	outerStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E2E8F0"))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8"))

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 1).
			Foreground(lipgloss.Color("#E2E8F0"))

	cardTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7DD3FC")).
			Bold(true)

	cardBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8FAFC")).
			Bold(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 1).
			Foreground(lipgloss.Color("#CBD5E1"))

	panelHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7DD3FC")).
				Bold(true)

	panelBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CBD5E1"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FCA5A5")).
			Bold(true)

	statusReadyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#34D399")).
				Bold(true)

	statusBusyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)

	statusErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FCA5A5")).
				Bold(true)

	statusMutedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94A3B8"))

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7DD3FC")).
			Bold(true)

	cardWidth = 28
)
