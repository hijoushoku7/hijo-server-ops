package gclog

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	gcIDPattern   = regexp.MustCompile(`GC\(([0-9]+)\)`)
	uptimePattern = regexp.MustCompile(
		`\[([0-9]+(?:\.[0-9]+)?)s\]`,
	)
	transitionPattern = regexp.MustCompile(
		`(?:^|\s)([0-9]+(?:\.[0-9]+)?)([KMGT]?)B?` +
			`(?:\([0-9]+%\))?->` +
			`([0-9]+(?:\.[0-9]+)?)([KMGT]?)B?` +
			`(?:\([0-9]+%\))?` +
			`(?:\(([0-9]+(?:\.[0-9]+)?)([KMGT]?)B?\))?`,
	)
	durationPattern = regexp.MustCompile(
		`([0-9]+(?:\.[0-9]+)?)(ms|s)\s*$`,
	)
)

type Bytes struct {
	Value     uint64
	Available bool
}

type Duration struct {
	Value     time.Duration
	Available bool
}

type Event struct {
	ID       uint64
	Uptime   Duration
	Before   Bytes
	After    Bytes
	Capacity Bytes
	Pause    Duration
}

func Parse(line string) (Event, bool) {
	idMatch := gcIDPattern.FindStringSubmatch(line)
	if idMatch == nil {
		return Event{}, false
	}
	id, err := strconv.ParseUint(idMatch[1], 10, 64)
	if err != nil {
		return Event{}, false
	}

	event := Event{ID: id}
	if match := uptimePattern.FindStringSubmatch(line); match != nil {
		if value, ok := parseDuration(match[1], "s"); ok {
			event.Uptime = Duration{Value: value, Available: true}
		}
	}

	transition := transitionPattern.FindStringSubmatch(line)
	if transition != nil {
		event.Before = parseBytes(transition[1], transition[2])
		event.After = parseBytes(transition[3], transition[4])
		if transition[5] != "" {
			event.Capacity = parseBytes(transition[5], transition[6])
		}
	}

	if strings.Contains(line, " Pause ") {
		if match := durationPattern.FindStringSubmatch(line); match != nil {
			if value, ok := parseDuration(match[1], match[2]); ok {
				event.Pause = Duration{Value: value, Available: true}
			}
		}
	}

	if !event.After.Available && !event.Pause.Available {
		return Event{}, false
	}
	return event, true
}

func parseBytes(number, unit string) Bytes {
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || value < 0 {
		return Bytes{}
	}

	multiplier := float64(1)
	switch unit {
	case "K":
		multiplier = 1 << 10
	case "M":
		multiplier = 1 << 20
	case "G":
		multiplier = 1 << 30
	case "T":
		multiplier = 1 << 40
	case "":
	default:
		return Bytes{}
	}
	value *= multiplier
	if value > math.MaxUint64 {
		return Bytes{}
	}
	return Bytes{Value: uint64(math.Round(value)), Available: true}
}

func parseDuration(number, unit string) (time.Duration, bool) {
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	switch unit {
	case "ms":
		value *= float64(time.Millisecond)
	case "s":
		value *= float64(time.Second)
	default:
		return 0, false
	}
	if value > float64(math.MaxInt64) {
		return 0, false
	}
	return time.Duration(math.Round(value)), true
}
