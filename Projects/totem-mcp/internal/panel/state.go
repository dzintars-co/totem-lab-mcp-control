// Package panel stores confirmed state and a bounded, cursor-addressable event history.
package panel

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type State struct {
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

// Reading also preserves the ADC samples before normalization.
type Reading struct {
	State      State
	Raw        [3]uint16
	Seq        uint32
	Generation uint64
}

type Event struct {
	ID         uint64    `json:"id"`
	Time       time.Time `json:"time"`
	Kind       string    `json:"kind"`
	Generation uint64    `json:"generation"`
	Sequence   uint32    `json:"sequence"`
	State      *State    `json:"state,omitempty"`
	PotRaw     [3]uint16 `json:"pot_raw"`
	Message    string    `json:"message,omitempty"`
	Lost       uint32    `json:"lost,omitempty"`
}

type View struct {
	State         *State     `json:"state"`
	PotRaw        [3]uint16  `json:"pot_raw"`
	Valid         bool       `json:"valid"`
	Status        string     `json:"status"`
	Port          string     `json:"port"`
	Baud          int        `json:"baud"`
	Generation    uint64     `json:"generation"`
	Sequence      uint32     `json:"sequence"`
	LastUpdate    *time.Time `json:"last_update"`
	LastError     string     `json:"last_error,omitempty"`
	StreamID      string     `json:"stream_id"`
	FirstEventID  uint64     `json:"first_event_id"`
	LastEventID   uint64     `json:"last_event_id"`
	HistoryLost   bool       `json:"history_lost"`
	EvictedEvents uint64     `json:"evicted_events"`
	Events        []Event    `json:"events,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	view     View
	events   []Event
	limit    int
	onChange func()
}

func New(port string, baud, historySize int) (*Store, error) {
	if historySize < 1 {
		return nil, fmt.Errorf("history size must be positive")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	return &Store{
		limit: historySize,
		view:  View{Status: "disconnected", Port: port, Baud: baud, StreamID: hex.EncodeToString(id[:])},
	}, nil
}

// SetOnChange must be called before the serial worker starts. The callback must not block.
func (s *Store) SetOnChange(fn func()) { s.onChange = fn }

func (s *Store) changed() {
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *Store) appendLocked(e Event) {
	s.view.LastEventID++
	e.ID, e.Time, e.Generation = s.view.LastEventID, time.Now().UTC(), s.view.Generation
	if len(s.events) == s.limit {
		copy(s.events, s.events[1:])
		s.events = s.events[:len(s.events)-1]
		s.view.EvictedEvents++
	}
	s.events = append(s.events, e)
	s.view.FirstEventID = s.events[0].ID
}

func (s *Store) Connecting() {
	s.mu.Lock()
	s.view.Generation++
	s.view.Status, s.view.Valid, s.view.State = "connecting", false, nil
	s.view.Sequence, s.view.LastUpdate, s.view.LastError = 0, nil, ""
	s.view.PotRaw = [3]uint16{}
	s.appendLocked(Event{Kind: "connection", Message: "awaiting a complete firmware reading"})
	s.mu.Unlock()
	s.changed()
}

func (s *Store) Disconnected(reason string) {
	s.mu.Lock()
	s.view.Status, s.view.Valid, s.view.LastError = "disconnected", false, reason
	// Retained data is explicitly invalid; it is never substituted for a fresh reading.
	s.appendLocked(Event{Kind: "connection", Message: reason})
	s.mu.Unlock()
	s.changed()
}

func (s *Store) Apply(r Reading) {
	s.mu.Lock()
	changed := !s.view.Valid || s.view.State == nil || *s.view.State != r.State || s.view.PotRaw != r.Raw
	state, now := r.State, time.Now().UTC()
	s.view.State, s.view.PotRaw, s.view.Sequence = &state, r.Raw, r.Seq
	s.view.LastUpdate, s.view.Status, s.view.Valid = &now, "connected", true
	if changed {
		s.appendLocked(Event{Kind: "state", Sequence: r.Seq, State: &state, PotRaw: r.Raw})
	}
	s.mu.Unlock()
	if changed {
		s.changed()
	}
}

func (s *Store) Report(kind, message string, lost uint32) {
	s.mu.Lock()
	s.view.LastError = message
	s.appendLocked(Event{Kind: kind, Message: message, Lost: lost})
	s.mu.Unlock()
	s.changed()
}

// View returns copies. A cursor is meaningful only within the same StreamID.
// nil after and history=false returns state alone; history=true returns retained events.
func (s *Store) View(after *uint64, streamID string, history bool) View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.view
	if v.State != nil {
		state := *v.State
		v.State = &state
	}
	if v.LastUpdate != nil {
		t := *v.LastUpdate
		v.LastUpdate = &t
	}
	v.HistoryLost = streamID != "" && streamID != v.StreamID
	if after != nil {
		v.HistoryLost = v.HistoryLost || *after > v.LastEventID ||
			(v.FirstEventID > 0 && *after < v.FirstEventID-1)
	}
	if history || after != nil {
		for _, e := range s.events {
			if after != nil && !v.HistoryLost && e.ID <= *after {
				continue
			}
			if e.State != nil {
				st := *e.State
				e.State = &st
			}
			v.Events = append(v.Events, e)
		}
	}
	return v
}
