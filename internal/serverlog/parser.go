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
	SentByHSO       bool
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

// shutdownNotice はシャットダウン処理そのものが出す行。/stop のコマンド
// フィードバック `Stopping the server`（"the" が入る）とは別の行で、止まり
// 方に関係なく整然と畳まれるときは必ず出る。
const shutdownNotice = "Stopping server"

// crashNotices はサーバーがクラッシュとして畳まれたときに出る行。
// クラッシュでもシャットダウン処理は走るので、これが出ていたかどうかが
// 整然停止とクラッシュの唯一の区別になる。終了コードは当てにならない
// （クラッシュレポートを書いた後、正常終了の経路を通って 0 で終わる）。
var crashNotices = []string{
	"Encountered an unexpected exception",
	"This crash report has been saved to",
}

// IsShutdownStart はサーバーが整然と畳まれ始めた行かを返す。
//
// 誰が止めたかは見ない。/stop のコマンドフィードバックを見る手もあるが、
// プレイヤーがワールドで実行した場合の `[名前: Stopping the server]` は
// gamerule logAdminCommands に依存し、切られている環境では出ない。
// シャットダウン処理自身が出すこの行はゲームルールの影響を受けない。
func IsShutdownStart(entry Entry) bool {
	return entry.Kind == KindOther && strings.TrimSpace(entry.Message) == shutdownNotice
}

// IsCrashNotice はクラッシュとして畳まれたと分かる行かを返す。チャットで
// 同じ文字列を打たれても拾わないよう、サーバー自身の行だけを見る。
func IsCrashNotice(entry Entry) bool {
	if entry.Kind != KindOther {
		return false
	}
	message := strings.TrimSpace(entry.Message)
	for _, notice := range crashNotices {
		if strings.HasPrefix(message, notice) {
			return true
		}
	}
	return false
}

func SentCommand(command string) Entry {
	command = strings.TrimRight(command, "\r\n")
	return Entry{
		Kind:      KindCommand,
		SentByHSO: true,
		Message:   command,
		Command:   command,
	}
}

// Whisper はささやきコマンドから宛先と本文を取り出す。
// 本文の前後の空白だけを除き、本文中の空白はそのまま残す。
func Whisper(command string) (target, body string, ok bool) {
	command = strings.TrimSpace(command)
	command = strings.TrimPrefix(command, "/")

	nameEnd := strings.IndexAny(command, " \t\r\n")
	if nameEnd < 0 {
		return "", "", false
	}
	switch command[:nameEnd] {
	case "tell", "msg", "w":
	default:
		return "", "", false
	}

	remainder := strings.TrimLeft(command[nameEnd:], " \t\r\n")
	targetEnd := strings.IndexAny(remainder, " \t\r\n")
	if targetEnd < 0 {
		return "", "", false
	}
	target = remainder[:targetEnd]
	body = strings.TrimSpace(remainder[targetEnd:])
	if target == "" || body == "" {
		return "", "", false
	}
	return target, body, true
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
