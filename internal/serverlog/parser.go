package serverlog

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Kind uint8

const (
	KindOther Kind = iota
	KindChat
	KindCommand
	KindPlayerJoin
	KindPlayerLeave
	KindLag
	KindIgnored
)

func (kind Kind) String() string {
	switch kind {
	case KindChat:
		return "chat"
	case KindCommand:
		return "command"
	case KindPlayerJoin:
		return "player_join"
	case KindPlayerLeave:
		return "player_leave"
	case KindLag:
		return "lag"
	case KindIgnored:
		return "ignored"
	default:
		return "other"
	}
}

type Lag struct {
	Behind      time.Duration
	BehindKnown bool
	TicksBehind uint64
	TicksKnown  bool
}

type Entry struct {
	Kind    Kind
	Raw     string
	Message string
	Player  string
	Chat    string
	Command string
	Reason  string
	Lag     Lag
}

var (
	ansiPattern = regexp.MustCompile(
		"\x1b\\[[0-?]*[ -/]*[@-~]",
	)
	logPrefixPattern = regexp.MustCompile(
		`^\[[^\]\r\n]+\]\s+` +
			`\[[^\]\r\n]+/(?:TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\]` +
			`(?:\s+\[[^\]\r\n]+\])*\s*:\s?(.*)$`,
	)
	chatPattern = regexp.MustCompile(
		`^(?:\[Not Secure\]\s+)?<([^<>\s]+)>\s?(.*)$`,
	)
	commandPattern = regexp.MustCompile(
		`^(\S+)\s+issued server command:\s+(.+)$`,
	)
	joinPattern = regexp.MustCompile(
		`^(\S+)\s+joined the game$`,
	)
	leavePattern = regexp.MustCompile(
		`^(\S+)\s+left the game$`,
	)
	lostConnectionPattern = regexp.MustCompile(
		`^(\S+)\s+lost connection:\s*(.*)$`,
	)
	modernLagPattern = regexp.MustCompile(
		`Running\s+([0-9]+)ms\s+or\s+([0-9]+)\s+ticks?\s+behind`,
	)
	legacyLagPattern = regexp.MustCompile(
		`Running\s+([0-9]+)ms\s+behind,\s+skipping\s+` +
			`([0-9]+)\s+tick(?:s|\(s\))?`,
	)
)

func Parse(line string) Entry {
	raw := strings.TrimRight(line, "\r\n")
	normalized := ansiPattern.ReplaceAllString(raw, "")
	message := extractMessage(normalized)
	entry := Entry{
		Kind:    KindOther,
		Raw:     raw,
		Message: message,
	}

	if strings.HasPrefix(strings.TrimSpace(message), "Picked up JAVA_TOOL_OPTIONS:") {
		entry.Kind = KindIgnored
		return entry
	}

	if match := chatPattern.FindStringSubmatch(message); match != nil {
		entry.Kind = KindChat
		entry.Player = match[1]
		entry.Chat = match[2]
		return entry
	}
	if match := commandPattern.FindStringSubmatch(message); match != nil {
		entry.Kind = KindCommand
		entry.Player = match[1]
		entry.Command = match[2]
		return entry
	}
	if match := joinPattern.FindStringSubmatch(message); match != nil {
		entry.Kind = KindPlayerJoin
		entry.Player = match[1]
		return entry
	}
	if match := leavePattern.FindStringSubmatch(message); match != nil {
		entry.Kind = KindPlayerLeave
		entry.Player = match[1]
		return entry
	}
	if match := lostConnectionPattern.FindStringSubmatch(message); match != nil {
		entry.Kind = KindPlayerLeave
		entry.Player = match[1]
		entry.Reason = match[2]
		return entry
	}
	if strings.Contains(message, "Can't keep up!") {
		entry.Kind = KindLag
		entry.Lag = parseLag(message)
	}
	return entry
}

func SentCommand(command string) Entry {
	command = strings.TrimRight(command, "\r\n")
	return Entry{
		Kind:    KindCommand,
		Message: command,
		Command: command,
	}
}

func extractMessage(line string) string {
	match := logPrefixPattern.FindStringSubmatch(line)
	if match == nil {
		return line
	}
	return match[1]
}

func parseLag(message string) Lag {
	match := modernLagPattern.FindStringSubmatch(message)
	if match == nil {
		match = legacyLagPattern.FindStringSubmatch(message)
	}
	if match == nil {
		return Lag{}
	}

	var lag Lag
	if milliseconds, ok := parseUint(match[1]); ok &&
		milliseconds <= uint64(math.MaxInt64/int64(time.Millisecond)) {
		lag.Behind = time.Duration(milliseconds) * time.Millisecond
		lag.BehindKnown = true
	}
	if ticks, ok := parseUint(match[2]); ok {
		lag.TicksBehind = ticks
		lag.TicksKnown = true
	}
	return lag
}

func parseUint(value string) (uint64, bool) {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return parsed, err == nil
}
