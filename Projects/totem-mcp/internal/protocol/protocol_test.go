package protocol

import (
	"strings"
	"testing"
)

const sample = "STATE;V=1;SEQ=4294967295;SW1=1;SW2=0;SW3=1;B1=0;B2=1;B3=0;P1=0;P2=512;P3=1023;ENC=-2147483648;EB=0;RGB=#00ff00;RELAY=1"

func TestStatePreservesRangesAndRawADC(t *testing.T) {
	f, err := Parse(sample + "\r")
	if err != nil {
		t.Fatal(err)
	}
	s := f.Reading.State
	if f.Reading.Seq != 4294967295 || s.Encoder != -2147483648 || s.RGB != "#00FF00" || s.Button2 != 1 || s.Relay != 1 {
		t.Fatalf("wrong decoded state: %+v", f)
	}
	if f.Reading.Raw != [3]uint16{0, 512, 1023} || s.Pot1 != 0 || s.Pot2 != 512.0/1023 || s.Pot3 != 1 {
		t.Fatal("ADC values were rounded, clamped or normalized incorrectly")
	}
}

func TestMalformedFramesAreRejected(t *testing.T) {
	bad := []string{
		sample + ";SW1=0",
		strings.Replace(sample, ";EB=0", "", 1),
		strings.Replace(sample, "P3=1023", "P3=1024", 1),
		strings.Replace(sample, "SW1=1", "SW1=2", 1),
		strings.Replace(sample, "RELAY=1", "RELAY=2", 1),
		strings.Replace(sample, "RGB=#00ff00", "RGB=#00XX00", 1),
		strings.Replace(sample, "ENC=-2147483648", "ENC=2147483648", 1),
		strings.Replace(sample, "V=1", "V=2", 1),
		strings.Replace(sample, "SEQ=4294967295", "SEQ=4294967296", 1),
		"ACK" + strings.TrimPrefix(sample, "STATE") + ";ID=0",
		"GAP;V=1;LOST=0",
		"READY;V=1;UNEXPECTED=1",
		strings.Repeat("x", MaxLine+1),
	}
	for _, line := range bad {
		if _, err := Parse(line); err == nil {
			t.Errorf("accepted malformed frame: %s", line)
		}
	}
}

func TestCommandsAreUnambiguous(t *testing.T) {
	line, err := Command("RGB", 42, "#12abEF")
	if err != nil || line != "RGB;V=1;ID=42;VALUE=#12ABEF\n" {
		t.Fatalf("unexpected command %q: %v", line, err)
	}
	for _, tc := range []struct {
		kind, value string
		id          uint32
	}{
		{"RGB", "#fff", 1}, {"RGB", "#000000\nRELAY=1", 1},
		{"RELAY", "2", 1}, {"GET", "extra", 1}, {"GET", "", 0}, {"OTHER", "", 1},
	} {
		if _, err := Command(tc.kind, tc.id, tc.value); err == nil {
			t.Errorf("accepted invalid command: %+v", tc)
		}
	}
}
