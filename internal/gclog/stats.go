package gclog

import "time"

type Rate struct {
	PerMinute float64
	Available bool
}

type Count struct {
	Value     uint64
	Available bool
}

type Stats struct {
	Collections Count
	PostGC      Bytes
	LastPause   Duration
	TotalPause  time.Duration

	highestCollectionID uint64
	hasCollectionID     bool
	firstTimedUptime    time.Duration
	lastTimedUptime     time.Duration
	timedCollections    uint64
	hasFirstTimedUptime bool
}

func (s *Stats) Add(event Event) {
	if !s.hasCollectionID || event.ID > s.highestCollectionID {
		s.Collections.Value++
		s.Collections.Available = true
		s.highestCollectionID = event.ID
		s.hasCollectionID = true

		if event.Uptime.Available {
			if !s.hasFirstTimedUptime {
				s.firstTimedUptime = event.Uptime.Value
				s.hasFirstTimedUptime = true
			}
			s.lastTimedUptime = event.Uptime.Value
			s.timedCollections++
		}
	}
	if event.After.Available {
		s.PostGC = event.After
	}
	if event.Pause.Available {
		s.LastPause = event.Pause
		s.TotalPause += event.Pause.Value
	}
}

func (s Stats) Frequency() Rate {
	if s.timedCollections < 2 || s.lastTimedUptime <= s.firstTimedUptime {
		return Rate{}
	}
	elapsed := s.lastTimedUptime - s.firstTimedUptime
	return Rate{
		PerMinute: float64(s.timedCollections-1) /
			elapsed.Minutes(),
		Available: true,
	}
}
