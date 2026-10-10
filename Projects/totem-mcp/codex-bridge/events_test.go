package main

import (
	"errors"
	"reflect"
	"testing"
)

func baselineDoc() document {
	return document{StreamID: "test", Valid: true, Status: "connected", Generation: 1, State: &state{RGB: "#000000"}, FirstEventID: 1, LastEventID: 1}
}

func TestPressReleaseAndDuplicateNotifications(t *testing.T) {
	c := cursor{potStep: 2}
	c.baseline(baselineDoc())
	d := baselineDoc()
	d.LastEventID = 3
	d.Events = []panelEvent{{ID: 3, Kind: "state", Generation: 1, State: &state{}}, {ID: 2, Kind: "state", Generation: 1, State: &state{Button1: 1}}}
	var got []change
	emit := func(x change) error { got = append(got, x); return nil }
	if err := c.process(d, emit); err != nil {
		t.Fatal(err)
	}
	if err := c.process(d, emit); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].State.Button1 != 1 || got[1].State.Button1 != 0 {
		t.Fatalf("lost ordered transitions: %+v", got)
	}
	if !reflect.DeepEqual(got[0].Fields, []string{"button1", "rgb"}) {
		t.Fatal(got[0].Fields)
	}
}

func TestReconnectAndLostHistoryNeverInventPress(t *testing.T) {
	for _, mode := range []string{"stream", "history", "generation", "gap"} {
		t.Run(mode, func(t *testing.T) {
			c := cursor{}
			c.baseline(baselineDoc())
			d := baselineDoc()
			d.State = &state{Button1: 1}
			d.LastEventID = 2
			switch mode {
			case "stream":
				d.StreamID = "new"
			case "history":
				d.FirstEventID = 10
				d.LastEventID = 10
			case "generation":
				d.Events = []panelEvent{{ID: 2, Kind: "state", Generation: 2, State: d.State}}
			case "gap":
				d.LastEventID = 3
				d.Events = []panelEvent{{ID: 2, Kind: "gap", Lost: 3}, {ID: 3, Kind: "state", Generation: 1, State: d.State}}
			}
			var got []change
			if err := c.process(d, func(x change) error { got = append(got, x); return nil }); err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 {
				t.Fatal("missing resync notification")
			}
			for _, x := range got {
				if len(x.Fields) != 0 {
					t.Fatalf("invented transition: %+v", x)
				}
			}
		})
	}
}

func TestPotThresholdAccumulatesAndDigitalIsImmediate(t *testing.T) {
	c := cursor{potStep: 2}
	c.baseline(baselineDoc())
	d := baselineDoc()
	d.LastEventID = 4
	d.Events = []panelEvent{
		{ID: 2, Generation: 1, Kind: "state", State: &state{Pot1: 0.01, RGB: "#000000"}},
		{ID: 3, Generation: 1, Kind: "state", State: &state{Pot1: 0.015, Button2: 1, RGB: "#000000"}},
		{ID: 4, Generation: 1, Kind: "state", State: &state{Pot1: 0.02, Button2: 1, RGB: "#000000"}},
	}
	var got []change
	if err := c.process(d, func(x change) error { got = append(got, x); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !reflect.DeepEqual(got[0].Fields, []string{"button2"}) || !reflect.DeepEqual(got[1].Fields, []string{"pot1"}) {
		t.Fatalf("threshold: %+v", got)
	}
}

func TestOfflineAndIncompleteHistory(t *testing.T) {
	c := cursor{}
	c.baseline(baselineDoc())
	d := baselineDoc()
	d.Valid = false
	d.Status = "disconnected"
	d.LastEventID = 2
	d.Events = []panelEvent{{ID: 2, Kind: "connection", Message: "unplugged"}}
	if err := c.process(d, func(x change) error {
		if x.Valid || x.State != nil {
			t.Error("stale state marked valid")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d.LastEventID = 4
	d.Events = []panelEvent{{ID: 4, Kind: "state", State: &state{}}}
	if err := c.process(d, func(change) error { return nil }); err == nil {
		t.Fatal("incomplete history accepted")
	}
}

func TestQueueFailureDoesNotAdvanceCursor(t *testing.T) {
	c := cursor{}
	c.baseline(baselineDoc())
	d := baselineDoc()
	d.LastEventID = 2
	d.Events = []panelEvent{{ID: 2, Generation: 1, Kind: "state", State: &state{Button1: 1}}}
	if err := c.process(d, func(change) error { return errQueueFull }); !errors.Is(err, errQueueFull) {
		t.Fatal(err)
	}
	if c.id != 1 {
		t.Fatal("undelivered event acknowledged")
	}
}
