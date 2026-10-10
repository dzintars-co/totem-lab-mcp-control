package main

import (
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
