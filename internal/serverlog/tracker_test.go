package serverlog

import (
	"reflect"
	"testing"
)

func TestTrackerTracksPlayersAndLagEvents(t *testing.T) {
	var tracker Tracker
	entries := []Entry{
		Parse("[12:00:00] [Server thread/INFO]: zoe joined the game"),
		Parse("[12:00:01] [Server thread/INFO]: alice joined the game"),
		Parse("[12:00:02] [Server thread/INFO]: zoe joined the game"),
		Parse("[12:00:03] [Server thread/WARN]: Can't keep up!"),
		Parse("[12:00:04] [Server thread/INFO]: zoe left the game"),
	}
	for _, entry := range entries {
		tracker.Apply(entry)
	}

	if got, want := tracker.Players(), []string{"alice"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Players = %v, want %v", got, want)
	}
	if tracker.PlayerCount() != 1 {
		t.Fatalf("PlayerCount = %d", tracker.PlayerCount())
	}
	if tracker.LagEvents() != 1 {
		t.Fatalf("LagEvents = %d", tracker.LagEvents())
	}
}

func TestTrackerLostConnectionRemovesPlayer(t *testing.T) {
	var tracker Tracker
	tracker.Apply(Parse(
		"[12:00:00] [Server thread/INFO]: alice joined the game",
	))
	tracker.Apply(Parse(
		"[12:00:01] [Server thread/INFO]: " +
			"alice lost connection: Disconnected",
	))

	if tracker.PlayerCount() != 0 {
		t.Fatalf("Players = %v", tracker.Players())
	}
}

func TestTrackerPlayersReturnsCopy(t *testing.T) {
	var tracker Tracker
	tracker.Apply(Parse(
		"[12:00:00] [Server thread/INFO]: alice joined the game",
	))

	players := tracker.Players()
	players[0] = "changed"

	if got := tracker.Players(); !reflect.DeepEqual(got, []string{"alice"}) {
		t.Fatalf("Players = %v", got)
	}
}
