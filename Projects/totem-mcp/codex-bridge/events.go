package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

// These JSON names follow the existing Totem panel://events contract.
type state struct {
	Switch1       int     `json:"switch1"`
	Switch2       int     `json:"switch2"`
	Switch3       int     `json:"switch3"`
	Button1       int     `json:"button1"`
	Button2       int     `json:"button2"`
	Button3       int     `json:"button3"`
	Pot1          float64 `json:"pot1"`
	Pot2          float64 `json:"pot2"`
	Pot3          float64 `json:"pot3"`
	Encoder       int32   `json:"encoder"`
	EncoderButton int     `json:"encoder_button"`
	RGB           string  `json:"rgb"`
	Relay         int     `json:"relay"`
}

type panelEvent struct {
	ID         uint64    `json:"id"`
	Time       string    `json:"time"`
	Kind       string    `json:"kind"`
	Generation uint64    `json:"generation"`
	State      *state    `json:"state,omitempty"`
	PotRaw     [3]uint16 `json:"pot_raw"`
	Message    string    `json:"message,omitempty"`
	Lost       uint32    `json:"lost,omitempty"`
}

type document struct {
	Valid        bool         `json:"valid"`
	Status       string       `json:"status"`
	State        *state       `json:"state"`
	PotRaw       [3]uint16    `json:"pot_raw"`
	Port         string       `json:"port"`
	Baud         int          `json:"baud"`
	Generation   uint64       `json:"generation"`
	LastUpdate   *time.Time   `json:"last_update"`
	StreamID     string       `json:"stream_id"`
	FirstEventID uint64       `json:"first_event_id"`
	LastEventID  uint64       `json:"last_event_id"`
	HistoryLost  bool         `json:"history_lost"`
	Events       []panelEvent `json:"events"`
}

type change struct {
	StreamID string    `json:"stream_id"`
	EventID  uint64    `json:"event_id"`
	Time     string    `json:"time,omitempty"`
	Kind     string    `json:"kind"`
	Fields   []string  `json:"changed_fields,omitempty"`
	State    *state    `json:"state,omitempty"`
	PotRaw   [3]uint16 `json:"pot_raw"`
	Message  string    `json:"message,omitempty"`
	Valid    bool      `json:"valid"`
	Status   string    `json:"status"`
}

// Cursor survives transport reconnects in this process. A fresh process starts
// at the latest snapshot, so historic button presses never cause new actions.
type cursor struct {
	initialized    bool
	stream         string
	id, generation uint64
	previous       *state
	potAnchor      [3]float64
	potStep        float64
}

func (c *cursor) baseline(d document) {
	c.initialized, c.stream, c.id, c.generation = true, d.StreamID, d.LastEventID, d.Generation
	c.previous = nil
	if d.Valid && d.Status == "connected" && d.State != nil {
		v := *d.State
		c.previous = &v
		c.potAnchor = [3]float64{v.Pot1, v.Pot2, v.Pot3}
	}
}

func (c *cursor) process(d document, emit func(change) error) error {
	if d.StreamID == "" {
		return fmt.Errorf("empty stream_id")
	}
	if !c.initialized {
		c.baseline(d)
		return nil
	}
	if d.StreamID != c.stream || d.HistoryLost || d.LastEventID < c.id ||
		(d.FirstEventID > 0 && c.id < d.FirstEventID-1) {
		if err := emit(change{StreamID: d.StreamID, EventID: d.LastEventID, Kind: "resync", State: validState(d), PotRaw: d.PotRaw, Valid: d.Valid && d.Status == "connected" && d.State != nil, Status: d.Status, Message: "History unavailable: this is a fresh snapshot, not a button transition."}); err != nil {
			return err
		}
		c.baseline(d)
		return nil
	}
	events := append([]panelEvent(nil), d.Events...)
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	for _, e := range events {
		if e.ID <= c.id {
			continue
		}
		if e.ID != c.id+1 {
			return fmt.Errorf("incomplete history: expected event %d, got %d", c.id+1, e.ID)
		}
		x := change{StreamID: c.stream, EventID: e.ID, Time: e.Time, Kind: e.Kind, Message: e.Message, PotRaw: e.PotRaw}
		if e.Kind == "state" && e.State != nil {
			x.State, x.Valid, x.Status = e.State, true, "connected"
			if c.previous == nil || e.Generation != c.generation {
				x.Kind = "snapshot"
				c.potAnchor = [3]float64{e.State.Pot1, e.State.Pot2, e.State.Pot3}
			} else {
				x.Fields = c.fields(*c.previous, *e.State)
			}
			if x.Kind == "snapshot" || len(x.Fields) > 0 {
				if err := emit(x); err != nil {
					return err
				}
			}
			v := *e.State
			c.previous = &v
			c.generation = e.Generation
		} else {
			// Connection/GAP invalidates transition inference until a complete
			// state arrives. Never attach an old state to an offline event.
			if e.Kind == "connection" || e.Kind == "gap" {
				c.previous = nil
			}
			x.Status = "unknown"
			if err := emit(x); err != nil {
				return err
			}
		}
		c.id = e.ID
	}
	if c.id != d.LastEventID {
		return fmt.Errorf("incomplete history: processed %d, latest %d", c.id, d.LastEventID)
	}
	return nil
}

func validState(d document) *state {
	if d.Valid && d.Status == "connected" {
		return d.State
	}
	return nil
}

func (c *cursor) fields(a, b state) []string {
	var out []string
	for _, v := range []struct {
		name    string
		changed bool
	}{
		{"switch1", a.Switch1 != b.Switch1}, {"switch2", a.Switch2 != b.Switch2}, {"switch3", a.Switch3 != b.Switch3},
		{"button1", a.Button1 != b.Button1}, {"button2", a.Button2 != b.Button2}, {"button3", a.Button3 != b.Button3},
		{"encoder", a.Encoder != b.Encoder}, {"encoder_button", a.EncoderButton != b.EncoderButton},
		{"rgb", a.RGB != b.RGB}, {"relay", a.Relay != b.Relay},
	} {
		if v.changed {
			out = append(out, v.name)
		}
	}
	for i, v := range [3]float64{b.Pot1, b.Pot2, b.Pot3} {
		if delta := math.Abs(v-c.potAnchor[i]) * 100; delta > 0 && delta >= c.potStep {
			out = append(out, fmt.Sprintf("pot%d", i+1))
			c.potAnchor[i] = v
		}
	}
	return out
}

func encodeChanges(batch []change) (string, error) {
	b, err := json.Marshal(struct {
		Source string   `json:"source"`
		Events []change `json:"events"`
	}{"Totem MCP panel://events", batch})
	return string(b), err
}
