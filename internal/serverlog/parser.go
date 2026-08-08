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

type TimestampSource uint8

const (
	TimestampUnknown TimestampSource = iota
	TimestampLog
	TimestampReceived
)

type Lag struct {
	Behind      time.Duration
	BehindKnown bool
	TicksBehind uint64
	TicksKnown  bool
}

type Entry struct {
	Kind            Kind
	Timestamp       time.Time
	TimestampSource TimestampSource
	Raw             string
	Message         string
	Player          string
	Chat            string
	Command         string
	Reason          string
	Lag             Lag
}

var (
	ansiPattern = regexp.MustCompile(
		"\x1b\\[[0-?]*[ -/]*[@-~]",
	)
	logPrefixPattern = regexp.MustCompile(
		`^\[([^\]\r\n]+)\]\s+` +
			`\[[^\]\r\n]+/(?:TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\]` +
			`(?:\s+\[[^\]\r\n]+\])*\s*:\s?(.*)$`,
	)
	clockPattern = regexp.MustCompile(
		`(?:^|\s)([0-9]{2}:[0-9]{2}:[0-9]{2})(?:\.[0-9]+)?(?:$|\s)`,
	)
	chatPattern = regexp.MustCompile(
		`^(?:\[Not Secure\]\s+)?<([^<>\s]+)>\s?(.*)$`,
	)
	commandPattern = regexp.MustCompile(
		`^(\S+)\s+issued server command:\s+(.+)$`,
	)
	// バニラの sendCommandFeedback による `[実行者: 結果]` 形式。
	// プレイヤーが実行したコマンドはこの形でしかログに出ないことが多い。
	commandFeedbackPattern = regexp.MustCompile(
		`^\[([^\[\]:]+):\s*(.+)\]$`,
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
	message, timestamp, timestampSource := extractLogFields(normalized)
	entry := Entry{
		Kind:            KindOther,
		Timestamp:       timestamp,
		TimestampSource: timestampSource,
		Raw:             raw,
		Message:         message,
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
	if match := commandFeedbackPattern.FindStringSubmatch(message); match != nil {
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

// stoppingNotice は /stop のコマンドフィードバック。シャットダウン処理
// そのものが出す `Stopping server` とは別の行。
const stoppingNotice = "Stopping the server"

// IsStopping は「これから止まる」と告げる行かどうかを返す。コンソールから
// stop を打つと `Stopping the server` がそのまま出るが、プレイヤーが
// ワールドで /stop を実行した場合は sendCommandFeedback により
// `[名前: Stopping the server]` の形になる。どちらも意図された停止なので、
// クラッシュとして自動再起動に回さないための目印にする。
//
// プレイヤー実行の側は gamerule sendCommandFeedback に依存する。false の
// 環境では行が出ないので拾えない（従来どおりクラッシュ扱いになるだけで、
// 悪化はしない）。コンソール実行は gamerule の影響を受けない。
func IsStopping(entry Entry) bool {
	switch entry.Kind {
	case KindCommand:
		return strings.TrimSpace(entry.Command) == stoppingNotice
	case KindOther:
		return strings.TrimSpace(entry.Message) == stoppingNotice
	default:
		return false
	}
}

func SentCommand(command string) Entry {
	command = strings.TrimRight(command, "\r\n")
	return Entry{
		Kind:    KindCommand,
		Message: command,
		Command: command,
	}
}

func extractLogFields(line string) (string, time.Time, TimestampSource) {
	match := logPrefixPattern.FindStringSubmatch(line)
	if match == nil {
		return line, time.Time{}, TimestampUnknown
	}
	clock := clockPattern.FindStringSubmatch(match[1])
	if clock == nil {
		return match[2], time.Time{}, TimestampUnknown
	}
	timestamp, err := time.Parse("15:04:05", clock[1])
	if err != nil {
		return match[2], time.Time{}, TimestampUnknown
	}
	return match[2], timestamp, TimestampLog
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
