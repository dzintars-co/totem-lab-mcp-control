package panel

import "testing"

func testStore(t *testing.T, limit int) *Store {
	t.Helper()
	s, err := New("test-port", 115200, limit)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPressAndReleaseRemainInHistory(t *testing.T) {
	s := testStore(t, 16)
	s.Connecting()
	r := Reading{State: State{RGB: "#000000"}, Seq: 1}
	s.Apply(r)
	v := s.View(nil, "", false)
	cursor := v.LastEventID
	r.Seq, r.State.Button1 = 2, 1
	s.Apply(r)
	r.Seq, r.State.Button1 = 3, 0
	s.Apply(r)
	v = s.View(&cursor, v.StreamID, false)
	if v.HistoryLost || len(v.Events) != 2 || v.State.Button1 != 0 {
		t.Fatalf("missing transitions: %+v", v)
	}
	if v.Events[0].State.Button1 != 1 || v.Events[1].State.Button1 != 0 {
		t.Fatal("press and release were coalesced or reversed")
	}
	// A consumer cannot mutate the shared current state or historical snapshots.
	v.State.Button1, v.Events[0].State.Button1 = 99, 99
	again := s.View(&cursor, v.StreamID, false)
	if again.State.Button1 != 0 || again.Events[0].State.Button1 != 1 {
		t.Fatal("returned snapshots alias the store")
	}
}

func TestHistoryLossIsExplicit(t *testing.T) {
	s := testStore(t, 2)
	s.Connecting()
	cursor := uint64(0)
	for i := 1; i <= 3; i++ {
		s.Apply(Reading{Seq: uint32(i), State: State{Encoder: int32(i), RGB: "#000000"}})
	}
	v := s.View(&cursor, "", false)
	if !v.HistoryLost || v.EvictedEvents != 2 || len(v.Events) != 2 {
		t.Fatalf("overflow was not reported: %+v", v)
	}
	cursor = v.LastEventID
	if v := s.View(&cursor, "another-server-run", false); !v.HistoryLost || len(v.Events) != 2 {
		t.Fatal("cursor from a different server run was accepted")
	}
}

func TestDisconnectInvalidatesReadingAndReconnectClearsIt(t *testing.T) {
	s := testStore(t, 16)
	s.Connecting()
	s.Apply(Reading{State: State{RGB: "#00FF00", Relay: 1}, Seq: 1})
	s.Disconnected("USB removed")
	v := s.View(nil, "", false)
	if v.Valid || v.State == nil || v.State.Relay != 1 || v.Status != "disconnected" {
		t.Fatalf("stale reading was presented as current: %+v", v)
	}
	generation := v.Generation
	s.Connecting()
	v = s.View(nil, "", false)
	if v.Valid || v.State != nil || v.LastUpdate != nil || v.Generation != generation+1 {
		t.Fatal("new connection inherited a stale reading")
	}
}
