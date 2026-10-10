// Package protocol implements the project's serial protocol v1, not MCP framing.
package protocol

import (
	"fmt"
	"strconv"
	"strings"

	"totem-mcp/internal/panel"
)

const Version = "1"
const MaxLine = 240

type Frame struct {
	Kind    string
	ID      uint32
	Reading panel.Reading
	Code    string
	Lost    uint32
}

func Color(value string) (string, error) {
	if len(value) != 7 || value[0] != '#' {
		return "", fmt.Errorf("color must be #RRGGBB")
	}
	if _, err := strconv.ParseUint(value[1:], 16, 24); err != nil {
		return "", fmt.Errorf("color must be #RRGGBB")
	}
	return strings.ToUpper(value), nil
}

func Command(kind string, id uint32, value string) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("command ID must be nonzero")
	}
	switch kind {
	case "GET":
		if value != "" {
			return "", fmt.Errorf("GET takes no value")
		}
		return fmt.Sprintf("GET;V=1;ID=%d\n", id), nil
	case "RGB":
		color, err := Color(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("RGB;V=1;ID=%d;VALUE=%s\n", id, color), nil
	case "RELAY":
		if value != "0" && value != "1" {
			return "", fmt.Errorf("relay must be 0 or 1")
		}
		return fmt.Sprintf("RELAY;V=1;ID=%d;VALUE=%s\n", id, value), nil
	default:
		return "", fmt.Errorf("unknown command %q", kind)
	}
}

func Parse(line string) (Frame, error) {
	var f Frame
	if len(line) == 0 || len(line) > MaxLine {
		return f, fmt.Errorf("invalid line length")
	}
	line = strings.TrimSuffix(line, "\r")
	parts := strings.Split(line, ";")
	f.Kind = parts[0]
	kv := make(map[string]string, len(parts)-1)
	for _, token := range parts[1:] {
		key, value, ok := strings.Cut(token, "=")
		if !ok || key == "" || value == "" {
			return f, fmt.Errorf("invalid token %q", token)
		}
		if _, exists := kv[key]; exists {
			return f, fmt.Errorf("duplicate field %s", key)
		}
		kv[key] = value
	}
	if kv["V"] != Version {
		return f, fmt.Errorf("unsupported serial protocol version %q", kv["V"])
	}
	uintField := func(key string, bits int) (uint64, error) {
		value, exists := kv[key]
		if !exists || value == "" {
			return 0, fmt.Errorf("missing field %s", key)
		}
		for _, c := range value {
			if c < '0' || c > '9' {
				return 0, fmt.Errorf("invalid integer %s", key)
			}
		}
		n, err := strconv.ParseUint(value, 10, bits)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", key, err)
		}
		return n, nil
	}
	checkFields := func(want ...string) error {
		if len(kv) != len(want) {
			return fmt.Errorf("unexpected field count in %s", f.Kind)
		}
		for _, key := range want {
			if _, ok := kv[key]; !ok {
				return fmt.Errorf("missing field %s", key)
			}
		}
		return nil
	}
	switch f.Kind {
	case "READY":
		return f, checkFields("V")
	case "ERR":
		if err := checkFields("V", "ID", "CODE"); err != nil {
			return f, err
		}
		id, err := uintField("ID", 32)
		f.ID, f.Code = uint32(id), kv["CODE"]
		return f, err
	case "GAP":
		if err := checkFields("V", "LOST"); err != nil {
			return f, err
		}
		n, err := uintField("LOST", 32)
		if err == nil && n == 0 {
			err = fmt.Errorf("GAP must report a positive loss")
		}
		f.Lost = uint32(n)
		return f, err
	case "STATE", "ACK":
		fields := []string{"V", "SEQ", "SW1", "SW2", "SW3", "B1", "B2", "B3", "P1", "P2", "P3", "ENC", "EB", "RGB", "RELAY"}
		if f.Kind == "ACK" {
			fields = append(fields, "ID")
			id, err := uintField("ID", 32)
			if err != nil || id == 0 {
				return f, fmt.Errorf("invalid ACK ID")
			}
			f.ID = uint32(id)
		}
		if err := checkFields(fields...); err != nil {
			return f, err
		}
	default:
		return f, fmt.Errorf("unknown frame %q", f.Kind)
	}
	seq, err := uintField("SEQ", 32)
	if err != nil {
		return f, err
	}
	f.Reading.Seq = uint32(seq)
	state := &f.Reading.State
	bits := []struct {
		key    string
		target *int
	}{
		{"SW1", &state.Switch1}, {"SW2", &state.Switch2}, {"SW3", &state.Switch3},
		{"B1", &state.Button1}, {"B2", &state.Button2}, {"B3", &state.Button3},
		{"EB", &state.EncoderButton}, {"RELAY", &state.Relay},
	}
	for _, field := range bits {
		n, err := uintField(field.key, 1)
		if err != nil {
			return f, err
		}
		*field.target = int(n)
	}
	for i, key := range []string{"P1", "P2", "P3"} {
		n, err := uintField(key, 10)
		if err != nil {
			return f, err
		}
		f.Reading.Raw[i] = uint16(n)
	}
	state.Pot1, state.Pot2, state.Pot3 = float64(f.Reading.Raw[0])/1023, float64(f.Reading.Raw[1])/1023, float64(f.Reading.Raw[2])/1023
	encoder, err := strconv.ParseInt(kv["ENC"], 10, 32)
	if err != nil {
		return f, fmt.Errorf("invalid encoder: %w", err)
	}
	state.Encoder = int32(encoder)
	state.RGB, err = Color(kv["RGB"])
	return f, err
}
