package serverlog

import "sort"

type Tracker struct {
	players   map[string]struct{}
	lagEvents uint64
}

func (tracker *Tracker) Apply(entry Entry) {
	switch entry.Kind {
	case KindPlayerJoin:
		if tracker.players == nil {
			tracker.players = make(map[string]struct{})
		}
		tracker.players[entry.Player] = struct{}{}
	case KindPlayerLeave:
		delete(tracker.players, entry.Player)
	case KindLag:
		tracker.lagEvents++
	}
}

func (tracker *Tracker) Players() []string {
	players := make([]string, 0, len(tracker.players))
	for player := range tracker.players {
		players = append(players, player)
	}
	sort.Strings(players)
	return players
}

func (tracker *Tracker) PlayerCount() int {
	return len(tracker.players)
}

func (tracker *Tracker) LagEvents() uint64 {
	return tracker.lagEvents
}
