package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDashboardRendersLiveStateAndAgent(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 7, 28, 0, time.Local)
	m := initialDashboard(false)
	m.width, m.height, m.now = 120, 40, now
	m.doc = document{
		Valid: true, Status: "connected", Port: "/dev/ttyUSB0", Baud: 115200,
		LastEventID: 3635, PotRaw: [3]uint16{427, 194, 849},
		State: &state{Button1: 1, Pot1: .4174, Pot2: .1896, Pot3: .8299, Encoder: -2, RGB: "#00FF00", Relay: 1},
	}
	m.hasDocument = true
	m.mcpConnected = true
	m.agentAnswers = []string{"Poga 1: nospiesta (1)."}
	m.eventLines = []string{"15:07:28  Poga A=1"}

	v := m.View()
	plain := ansi.Strip(v.Content)
	for _, want := range []string{"TOTEM CONTROL", "CONNECTED", "/dev/ttyUSB0", "Poga A", "PRESSED", "427", "42%", "#00FF00", "Relejs", "Poga 1: nospiesta (1).", "15:07:28  Poga A=1"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, plain)
		}
	}
	if !v.AltScreen || v.WindowTitle != "Totem Control" {
		t.Fatalf("view settings: alt=%v title=%q", v.AltScreen, v.WindowTitle)
	}
}

func TestDashboardDisconnectInvalidatesCachedReading(t *testing.T) {
	m := initialDashboard(false)
	m.doc, m.hasDocument = baselineDoc(), true
	m.mcpConnected = true
	if !strings.Contains(ansi.Strip(m.renderHeader(100)), "  CONNECTED  ") {
		t.Fatal("valid live reading was not connected")
	}
	updated, _ := m.Update(mcpConnectionMsg{err: errors.New("stream closed")})
	m = updated.(dashboardModel)
	if !strings.Contains(ansi.Strip(m.renderHeader(100)), "DISCONNECTED") {
		t.Fatal("cached reading kept dashboard connected")
	}
	if !strings.Contains(ansi.Strip(m.renderPanel(70)), "nav aktuāls") || m.doc.State == nil {
		t.Fatal("last reading was not preserved and marked stale")
	}
	if !strings.Contains(ansi.Strip(m.renderAgent(70)), "stream closed") {
		t.Fatal("connection error was not displayed")
	}
	updated, _ = m.Update(documentMsg(baselineDoc()))
	m = updated.(dashboardModel)
	if !strings.Contains(ansi.Strip(m.renderHeader(100)), "DISCONNECTED") {
		t.Fatal("a reading alone claimed an active subscription")
	}
	updated, _ = m.Update(mcpConnectionMsg{connected: true})
	m = updated.(dashboardModel)
	if !strings.Contains(ansi.Strip(m.renderHeader(100)), "  CONNECTED  ") || m.mcpError != "" {
		t.Fatal("subscription recovery did not restore connection status")
	}
	updated, _ = m.Update(documentMsg(document{Status: "disconnected"}))
	m = updated.(dashboardModel)
	if !strings.Contains(ansi.Strip(m.renderHeader(100)), "DISCONNECTED") {
		t.Fatal("MCP connection masked a hardware disconnection")
	}
}

func TestDashboardFitsTerminalAndScrollsAllContent(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {70, 24}, {40, 12}, {24, 8}, {12, 5}, {1, 1}, {80, 2}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := initialDashboard(false)
			m.width, m.height = size[0], size[1]
			m.doc, m.hasDocument, m.mcpConnected = baselineDoc(), true, true
			m.agentAnswers = []string{strings.Repeat("Ābols 界 🌿 atbilde ", 60) + "ATBILDES_BEIGAS"}
			m.bridgeLog = "ŽURNĀLA_BEIGAS"
			var pages strings.Builder
			for {
				v := m.View()
				rows := strings.Split(v.Content, "\n")
				if len(rows) > m.height {
					t.Fatalf("view has %d rows in a %d-row terminal", len(rows), m.height)
				}
				for _, row := range rows {
					if width := ansi.StringWidth(row); width > m.width {
						t.Fatalf("row width %d exceeds terminal width %d: %q", width, m.width, row)
					}
				}
				pages.WriteString(ansi.Strip(v.Content))
				if m.scrollOffset == m.maxScroll() || m.viewportHeight() == 0 {
					break
				}
				updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
				m = updated.(dashboardModel)
			}
			if m.width >= 24 && m.viewportHeight() > 0 {
				for _, want := range []string{"Relejs", "ATBILDES_BEIGAS", "ŽURNĀLA_BEIGAS"} {
					if !strings.Contains(pages.String(), want) {
						t.Fatalf("scrolling never revealed %q", want)
					}
				}
			}
		})
	}
}

func TestDashboardScrollControlsAndResize(t *testing.T) {
	m := initialDashboard(false)
	m.width, m.height = 70, 12
	m.doc, m.hasDocument, m.mcpConnected = baselineDoc(), true, true
	m.agentAnswers = []string{strings.Repeat("Atbilde\n", 50)}
	for _, tc := range []struct {
		msg  tea.Msg
		want int
	}{
		{tea.KeyPressMsg{Code: tea.KeyDown}, 1},
		{tea.KeyPressMsg{Code: tea.KeyUp}, 0},
		{tea.KeyPressMsg{Code: tea.KeyPgDown}, 10},
		{tea.KeyPressMsg{Code: tea.KeyPgUp}, 0},
		{tea.MouseWheelMsg{Button: tea.MouseWheelDown}, 3},
		{tea.MouseWheelMsg{Button: tea.MouseWheelUp}, 0},
		{tea.KeyPressMsg{Code: tea.KeyEnd}, m.maxScroll()},
		{tea.KeyPressMsg{Code: tea.KeyHome}, 0},
	} {
		updated, _ := m.Update(tc.msg)
		m = updated.(dashboardModel)
		if m.scrollOffset != tc.want {
			t.Fatalf("%T: scroll=%d want=%d", tc.msg, m.scrollOffset, tc.want)
		}
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = updated.(dashboardModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 200})
	m = updated.(dashboardModel)
	if m.scrollOffset != 0 {
		t.Fatal("resize left the viewport beyond the remaining content")
	}
}

func TestDashboardUpdatesAndSmallLayout(t *testing.T) {
	m := initialDashboard(true)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	if cmd != nil {
		t.Fatal("resize returned command")
	}
	m = updated.(dashboardModel)
	updated, _ = m.Update(documentMsg(document{Valid: false, Status: "disconnected", Port: "/dev/ttyUSB0"}))
	m = updated.(dashboardModel)
	updated, _ = m.Update(changeMsg(change{Kind: "connection", Message: "unplugged"}))
	m = updated.(dashboardModel)
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"DISCONNECTED", "DRY RUN", "CONNECTION: unplugged"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("small dashboard missing %q:\n%s", want, plain)
		}
	}
}

func TestDescribeChangeUsesHardwareNamesAndValues(t *testing.T) {
	c := change{Time: "2026-10-07T12:04:23Z", Fields: []string{"button1", "pot3", "relay"}, State: &state{Button1: 0, Pot3: .83, Relay: 1}, PotRaw: [3]uint16{0, 0, 849}}
	got := describeChange(c)
	for _, want := range []string{"Poga A=0", "Pot C=849/83%", "Relejs=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("description missing %q: %s", want, got)
		}
	}
}

func TestRGBChannels(t *testing.T) {
	if r, g, b := rgbChannels("#12ABEF"); r != 0x12 || g != 0xab || b != 0xef {
		t.Fatalf("rgb = %d,%d,%d", r, g, b)
	}
	if r, g, b := rgbChannels("invalid"); r != 0 || g != 0 || b != 0 {
		t.Fatal("invalid RGB accepted")
	}
}
