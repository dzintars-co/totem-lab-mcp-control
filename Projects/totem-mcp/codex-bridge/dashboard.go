package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorGreen  = lipgloss.Color("#39E76F")
	colorYellow = lipgloss.Color("#F3E94E")
	colorBlue   = lipgloss.Color("#58A6FF")
	colorRed    = lipgloss.Color("#FF5F56")
	colorWhite  = lipgloss.Color("#F2F4F8")
	colorMuted  = lipgloss.Color("#8B949E")
	colorBorder = lipgloss.Color("#30363D")
	colorPanel  = lipgloss.Color("#0D1117")

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	labelStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	valueStyle   = lipgloss.NewStyle().Foreground(colorWhite)
	okStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorGreen)
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorYellow)
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorRed)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Background(colorPanel).Padding(0, 1)
)

type dashboardModel struct {
	width, height int
	scrollOffset  int
	now           time.Time
	doc           document
	hasDocument   bool
	mcpConnected  bool
	mcpError      string
	agentStatus   string
	agentBusy     bool
	agentAnswers  []string
	eventLines    []string
	bridgeLog     string
	err           string
	dry           bool
}

type documentMsg document
type mcpConnectionMsg struct {
	connected bool
	err       error
}
type changeMsg change
type agentBusyMsg struct{ count int }
type agentAnswerMsg string
type agentIdleMsg struct{}
type agentErrorMsg struct{ err error }
type bridgeLogMsg string
type dashboardTickMsg time.Time

func initialDashboard(dry bool) dashboardModel {
	status := "GAIDA NOTIKUMUS"
	if dry {
		status = "DRY RUN"
	}
	return dashboardModel{width: 100, height: 32, now: time.Now(), agentStatus: status, dry: dry}
}

func dashboardTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return dashboardTickMsg(t) })
}

func (m dashboardModel) Init() tea.Cmd { return dashboardTick() }

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.scrollOffset--
		case "down", "j":
			m.scrollOffset++
		case "pgup":
			m.scrollOffset -= max(1, m.viewportHeight())
		case "pgdown":
			m.scrollOffset += max(1, m.viewportHeight())
		case "home":
			m.scrollOffset = 0
		case "end":
			m.scrollOffset = m.maxScroll()
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollOffset -= 3
		case tea.MouseWheelDown:
			m.scrollOffset += 3
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case dashboardTickMsg:
		m.now = time.Time(msg)
		return m, dashboardTick()
	case documentMsg:
		m.doc, m.hasDocument = document(msg), true
	case mcpConnectionMsg:
		m.mcpConnected, m.mcpError = msg.connected, ""
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.mcpError = msg.err.Error()
		}
	case changeMsg:
		m.eventLines = appendRecent(m.eventLines, describeChange(change(msg)), 7)
	case agentBusyMsg:
		m.agentBusy = true
		m.agentStatus = fmt.Sprintf("APSTRĀDĀ %d NOTIKUMU(-US)", msg.count)
	case agentAnswerMsg:
		m.agentAnswers = appendRecent(m.agentAnswers, string(msg), 5)
	case agentIdleMsg:
		m.agentBusy = false
		if m.dry {
			m.agentStatus = "DRY RUN"
		} else {
			m.agentStatus = "GAIDA NOTIKUMUS"
		}
	case agentErrorMsg:
		m.agentBusy = false
		m.agentStatus = "KĻŪDA — RESTARTĒ SAVIENOJUMU"
		m.err = msg.err.Error()
	case bridgeLogMsg:
		m.bridgeLog = strings.TrimSpace(string(msg))
	}
	m.scrollOffset = min(max(0, m.scrollOffset), m.maxScroll())
	return m, nil
}

func (m dashboardModel) View() tea.View {
	width := m.contentWidth()
	lines := m.bodyLines()
	height := m.viewportHeight()
	offset := min(max(0, m.scrollOffset), max(0, len(lines)-height))
	visible := []string{m.renderHeader(width)}
	for i := 0; i < height; i++ {
		line := ""
		if offset+i < len(lines) {
			line = lines[offset+i]
		}
		visible = append(visible, line)
	}
	if m.height >= 3 {
		help := "↑↓ PgUp/PgDn Home/End • q: iziet"
		if width < 40 {
			help = "↑↓ • q: iziet"
		}
		if len(lines) > height {
			help = fmt.Sprintf("%d–%d/%d  %s", offset+1, min(offset+height, len(lines)), len(lines), help)
		}
		visible = append(visible, labelStyle.Render(ansi.Truncate(help, width, "")))
	}
	for i, line := range visible {
		visible[i] = ansi.Truncate(line, width, "")
		if m.width >= 60 {
			visible[i] = "  " + visible[i]
		}
	}
	view := tea.NewView(strings.Join(visible, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "Totem Control"
	return view
}

func (m dashboardModel) contentWidth() int {
	if m.width >= 60 {
		return m.width - 4
	}
	return max(1, m.width)
}

func (m dashboardModel) viewportHeight() int {
	if m.height < 3 {
		return max(0, m.height-1)
	}
	return m.height - 2 // Keep connection status and scroll controls visible.
}

func (m dashboardModel) maxScroll() int {
	return max(0, len(m.bodyLines())-m.viewportHeight())
}

func (m dashboardModel) bodyLines() []string {
	contentWidth := m.contentWidth()
	leftWidth := contentWidth
	rightWidth := contentWidth
	stacked := contentWidth < 104
	if !stacked {
		leftWidth = (contentWidth - 2) * 3 / 5
		rightWidth = contentWidth - leftWidth - 2
	}
	left := m.renderPanel(leftWidth)
	right := m.renderAgent(rightWidth)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	if stacked {
		body = lipgloss.JoinVertical(lipgloss.Left, left, right)
	}
	return strings.Split(ansi.Hardwrap(body, contentWidth, true), "\n")
}

func (m dashboardModel) renderHeader(width int) string {
	connected := m.mcpConnected && m.hasDocument && m.doc.Valid && m.doc.Status == "connected"
	status := errorStyle.Render("DISCONNECTED")
	if connected {
		status = okStyle.Render("CONNECTED")
	} else if m.mcpConnected && m.hasDocument && m.doc.Status != "" && m.doc.Status != "connected" {
		status = warnStyle.Render(strings.ToUpper(m.doc.Status))
	}
	port := m.doc.Port
	if port == "" {
		port = "—"
	}
	right := fmt.Sprintf("%s  %s  %s", m.now.Format("15:04:05"), status, labelStyle.Render(port))
	space := width - lipgloss.Width("TOTEM CONTROL") - lipgloss.Width(right)
	if space < 2 {
		compact := status + "  " + labelStyle.Render(port)
		if lipgloss.Width(compact) > width {
			compact = status
		}
		return ansi.Truncate(compact, width, "")
	}
	return titleStyle.Render("TOTEM CONTROL") + strings.Repeat(" ", space) + right
}

func (m dashboardModel) renderPanel(width int) string {
	if !m.hasDocument || m.doc.State == nil {
		return renderBox(width, sectionStyle.Render("PANELIS")+"\n\n"+warnStyle.Render("Gaida pirmo derīgo nolasījumu…"))
	}
	s := *m.doc.State
	var b strings.Builder
	b.WriteString(sectionStyle.Render("PANELIS"))
	b.WriteString("\n\n")
	if !m.mcpConnected || !m.doc.Valid || m.doc.Status != "connected" {
		b.WriteString(warnStyle.Render("Pēdējais nolasījums — nav aktuāls.") + "\n\n")
	}
	b.WriteString(groupTitle("ELEMENTS", "STATE", "VALUE"))
	b.WriteString("\n")
	b.WriteString(digitalRow("Slēdzis A", s.Switch1 == 1, "ON", "OFF", s.Switch1))
	b.WriteString("\n" + digitalRow("Slēdzis B", s.Switch2 == 1, "ON", "OFF", s.Switch2))
	b.WriteString("\n" + digitalRow("Slēdzis C", s.Switch3 == 1, "ON", "OFF", s.Switch3))
	b.WriteString("\n\n")
	b.WriteString(digitalRow("Poga A", s.Button1 == 1, "PRESSED", "RELEASED", s.Button1))
	b.WriteString("\n" + digitalRow("Poga B", s.Button2 == 1, "PRESSED", "RELEASED", s.Button2))
	b.WriteString("\n" + digitalRow("Poga C", s.Button3 == 1, "PRESSED", "RELEASED", s.Button3))
	b.WriteString("\n\n")
	b.WriteString(potRow("Pot A", s.Pot1, m.doc.PotRaw[0]))
	b.WriteString("\n" + potRow("Pot B", s.Pot2, m.doc.PotRaw[1]))
	b.WriteString("\n" + potRow("Pot C", s.Pot3, m.doc.PotRaw[2]))
	b.WriteString("\n\n")
	b.WriteString(textRow("Enkoders", strconv.FormatInt(int64(s.Encoder), 10), ""))
	b.WriteString("\n" + digitalRow("Enkodera poga", s.EncoderButton == 1, "PRESSED", "RELEASED", s.EncoderButton))
	b.WriteString("\n\n")
	r, g, blue := rgbChannels(s.RGB)
	b.WriteString(textRow("RGB R", strconv.Itoa(r), ""))
	b.WriteString("\n" + textRow("RGB G", strconv.Itoa(g), ""))
	b.WriteString("\n" + textRow("RGB B", strconv.Itoa(blue), ""))
	rgbPreview := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(s.RGB)).Render(s.RGB)
	b.WriteString("\n" + textRow("RGB", rgbPreview, ""))
	b.WriteString("\n\n" + digitalRow("Relejs", s.Relay == 1, "ON", "OFF", s.Relay))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render(fmt.Sprintf("PORT  %s   BAUD  %d   EVENT  %d", m.doc.Port, m.doc.Baud, m.doc.LastEventID)))
	return renderBox(width, b.String())
}

func (m dashboardModel) renderAgent(width int) string {
	inner := max(1, width-4)
	if width < 40 {
		inner = max(1, width)
	}
	status := okStyle.Render(m.agentStatus)
	if m.agentBusy {
		status = warnStyle.Render(m.agentStatus)
	}
	if m.err != "" {
		status = errorStyle.Render(m.agentStatus)
	}
	answers := "Vēl nav atbildes."
	if len(m.agentAnswers) > 0 {
		answers = strings.Join(m.agentAnswers, "\n\n")
	}
	events := "Vēl nav jaunu notikumu."
	if len(m.eventLines) > 0 {
		events = strings.Join(m.eventLines, "\n")
	}
	var b strings.Builder
	b.WriteString(sectionStyle.Render("CODEX CLI"))
	b.WriteString("\n" + status + "\n\n")
	b.WriteString(labelStyle.Render("ATBILDES") + "\n")
	b.WriteString(valueStyle.Width(inner).Render(answers))
	b.WriteString("\n\n" + labelStyle.Render("NOTIKUMI") + "\n")
	b.WriteString(valueStyle.Width(inner).Render(events))
	if m.mcpError != "" {
		b.WriteString("\n\n" + errorStyle.Render("MCP: "+m.mcpError))
	}
	if m.bridgeLog != "" {
		b.WriteString("\n\n" + labelStyle.Render(m.bridgeLog))
	}
	if m.err != "" {
		b.WriteString("\n\n" + errorStyle.Width(inner).Render(m.err))
	}
	return renderBox(width, b.String())
}

func renderBox(width int, content string) string {
	if width < 40 {
		return ansi.Wrap(content, max(1, width), "")
	}
	return boxStyle.Width(width - 2).Render(ansi.Wrap(content, width-4, ""))
}

func groupTitle(a, b, c string) string {
	return labelStyle.Render(padRight(a, 18) + " " + padRight(b, 12) + " " + c)
}

func digitalRow(name string, active bool, on, off string, raw int) string {
	stateText := off
	style := labelStyle
	if active {
		stateText, style = on, okStyle
	}
	return padRight(name, 18) + " " + padRight(style.Render(stateText), 12) + " " + strconv.Itoa(raw)
}

func textRow(name, value, suffix string) string {
	return padRight(name, 18) + " " + padRight(valueStyle.Render(value), 12) + " " + suffix
}

func potRow(name string, normalized float64, raw uint16) string {
	percent := int(normalized*100 + .5)
	return padRight(name, 18) + " " + padRight(strconv.Itoa(int(raw)), 12) + fmt.Sprintf(" %3d%%", percent)
}

func padRight(value string, width int) string {
	missing := width - lipgloss.Width(value)
	if missing <= 0 {
		return value
	}
	return value + strings.Repeat(" ", missing)
}

func rgbChannels(value string) (int, int, int) {
	if len(value) != 7 || value[0] != '#' {
		return 0, 0, 0
	}
	v, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return 0, 0, 0
	}
	return int(v >> 16), int(v >> 8 & 0xff), int(v & 0xff)
}

func appendRecent(items []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	items = append(items, value)
	if len(items) > limit {
		items = append([]string(nil), items[len(items)-limit:]...)
	}
	return items
}

func describeChange(c change) string {
	when := ""
	if t, err := time.Parse(time.RFC3339Nano, c.Time); err == nil {
		when = t.Local().Format("15:04:05") + "  "
	}
	if len(c.Fields) == 0 {
		if c.Message != "" {
			return when + strings.ToUpper(c.Kind) + ": " + c.Message
		}
		return when + strings.ToUpper(c.Kind)
	}
	parts := make([]string, 0, len(c.Fields))
	for _, field := range c.Fields {
		parts = append(parts, describeField(field, c.State, c.PotRaw))
	}
	return when + strings.Join(parts, ", ")
}

func describeField(field string, s *state, raw [3]uint16) string {
	if s == nil {
		return field
	}
	switch field {
	case "switch1":
		return fmt.Sprintf("Slēdzis A=%d", s.Switch1)
	case "switch2":
		return fmt.Sprintf("Slēdzis B=%d", s.Switch2)
	case "switch3":
		return fmt.Sprintf("Slēdzis C=%d", s.Switch3)
	case "button1":
		return fmt.Sprintf("Poga A=%d", s.Button1)
	case "button2":
		return fmt.Sprintf("Poga B=%d", s.Button2)
	case "button3":
		return fmt.Sprintf("Poga C=%d", s.Button3)
	case "pot1":
		return fmt.Sprintf("Pot A=%d/%d%%", raw[0], int(s.Pot1*100+.5))
	case "pot2":
		return fmt.Sprintf("Pot B=%d/%d%%", raw[1], int(s.Pot2*100+.5))
	case "pot3":
		return fmt.Sprintf("Pot C=%d/%d%%", raw[2], int(s.Pot3*100+.5))
	case "encoder":
		return fmt.Sprintf("Enkoders=%d", s.Encoder)
	case "encoder_button":
		return fmt.Sprintf("Enkodera poga=%d", s.EncoderButton)
	case "rgb":
		return "RGB=" + s.RGB
	case "relay":
		return fmt.Sprintf("Relejs=%d", s.Relay)
	default:
		return field
	}
}

type dashboardLogWriter struct{ send func(tea.Msg) }

func (w dashboardLogWriter) Write(p []byte) (int, error) {
	if w.send != nil {
		w.send(bridgeLogMsg(strings.TrimSpace(string(p))))
	}
	return len(p), nil
}

func runDashboard(parent context.Context, o options, _ io.Writer, _ io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	cwd, err := filepath.Abs(o.cwd)
	if err != nil {
		return err
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("cwd nav mape")
	}

	m := initialDashboard(o.dry)
	program := tea.NewProgram(m)
	oldLogWriter := log.Writer()
	log.SetOutput(dashboardLogWriter{send: program.Send})
	defer log.SetOutput(oldLogWriter)

	runner := &codexRunner{cli: o.cli, cwd: cwd, instruction: defaultInstruction, timeout: o.turnTimeout, out: io.Discard, stderr: io.Discard, onAnswer: func(answer string) { program.Send(agentAnswerMsg(answer)) }}
	if !o.dry {
		runner.threadFile, err = filepath.Abs(o.threadFile)
		if err != nil {
			return err
		}
		b, readErr := os.ReadFile(runner.threadFile)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		runner.thread = strings.TrimSpace(string(b))
		if strings.ContainsAny(runner.thread, "\r\n\t ") || strings.HasPrefix(runner.thread, "-") {
			return errors.New("invalid thread ID file")
		}
		if o.instructionFile != "" {
			b, readErr = os.ReadFile(o.instructionFile)
			if readErr != nil {
				return readErr
			}
			runner.instruction += "\n\nLietotāja papildu instrukcija:\n" + string(b)
		}
	}

	queue := make(chan change, 4096)
	emit := func(c change) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		select {
		case queue <- c:
			program.Send(changeMsg(c))
			return nil
		default:
			return errQueueFull
		}
	}

	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		err := listenPanel(ctx, o.endpoint, &cursor{potStep: o.potStep}, emit,
			func(d document) { program.Send(documentMsg(d)) },
			func(connected bool, err error) { program.Send(mcpConnectionMsg{connected: connected, err: err}) })
		if err != nil && !errors.Is(err, context.Canceled) {
			program.Send(agentErrorMsg{err: err})
		}
	}()
	go func() {
		defer workers.Done()
		runDashboardAgent(ctx, o, runner, queue, program)
	}()

	_, runErr := program.Run()
	cancel()
	workers.Wait()
	if errors.Is(runErr, tea.ErrInterrupted) {
		return nil
	}
	return runErr
}

func runDashboardAgent(ctx context.Context, o options, runner *codexRunner, queue <-chan change, program *tea.Program) {
	disabled := false
	for {
		var first change
		select {
		case <-ctx.Done():
			return
		case first = <-queue:
		}
		batch := []change{first}
		timer := time.NewTimer(o.batchWindow)
	collect:
		for len(batch) < 64 {
			select {
			case c := <-queue:
				batch = append(batch, c)
			case <-timer.C:
				break collect
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
		timer.Stop()
		if o.dry || disabled {
			continue
		}
		program.Send(agentBusyMsg{count: len(batch)})
		var stderr bytes.Buffer
		runner.stderr = &stderr
		if err := runner.deliver(ctx, batch); err != nil {
			if text := strings.TrimSpace(stderr.String()); text != "" {
				err = fmt.Errorf("%w: %s", err, text)
			}
			disabled = true
			program.Send(agentErrorMsg{err: err})
			continue
		}
		program.Send(agentIdleMsg{})
	}
}
