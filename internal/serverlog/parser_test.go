package serverlog

import (
	"testing"
	"time"
)

func TestParseVanillaChat(t *testing.T) {
	line := "[12:34:56] [Server thread/INFO]: <alice> hello world"

	entry := Parse(line)

	assertKind(t, entry, KindChat)
	if entry.Player != "alice" || entry.Chat != "hello world" {
		t.Fatalf("Entry = %#v", entry)
	}
	if entry.Message != "<alice> hello world" || entry.Raw != line {
		t.Fatalf("Entry = %#v", entry)
	}
}

func TestParseNotSecureChat(t *testing.T) {
	entry := Parse(
		"[12:34:56] [Server thread/INFO]: [Not Secure] <alice> hello",
	)

	assertKind(t, entry, KindChat)
	if entry.Player != "alice" || entry.Chat != "hello" {
		t.Fatalf("Entry = %#v", entry)
	}
}

func TestParseForgeCommand(t *testing.T) {
	entry := Parse(
		"[27Jul2026 12:34:56.123] [Server thread/INFO] " +
			"[net.minecraft.server.MinecraftServer/]: " +
			"alice issued server command: /time set day",
	)

	assertKind(t, entry, KindCommand)
	if entry.Player != "alice" || entry.Command != "/time set day" {
		t.Fatalf("Entry = %#v", entry)
	}
}

func TestParseCommandFeedback(t *testing.T) {
	tests := []struct {
		name    string
		message string
		player  string
		command string
	}{
		{
			name:    "gamemode",
			message: "[alice: Set own game mode to Creative Mode]",
			player:  "alice",
			command: "Set own game mode to Creative Mode",
		},
		{
			name:    "no space after colon",
			message: "[alice:changed gamemode]",
			player:  "alice",
			command: "changed gamemode",
		},
		{
			name:    "nested brackets",
			message: "[alice: Given [Diamond] * 1 to bob]",
			player:  "alice",
			command: "Given [Diamond] * 1 to bob",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := Parse("[12:34:56] [Server thread/INFO]: " + test.message)

			assertKind(t, entry, KindCommand)
			if entry.Player != test.player || entry.Command != test.command {
				t.Fatalf("Entry = %#v", entry)
			}
		})
	}
}

func TestParseCommandFeedbackDoesNotMatchChat(t *testing.T) {
	entry := Parse(
		"[12:34:56] [Server thread/INFO]: [Not Secure] <alice> hello",
	)

	assertKind(t, entry, KindChat)
}

func TestParsePlayerChanges(t *testing.T) {
	tests := []struct {
		name    string
		message string
		kind    Kind
		player  string
		reason  string
	}{
		{
			name:    "join",
			message: "alice joined the game",
			kind:    KindPlayerJoin,
			player:  "alice",
		},
		{
			name:    "leave",
			message: "alice left the game",
			kind:    KindPlayerLeave,
			player:  "alice",
		},
		{
			name:    "lost connection",
			message: "alice lost connection: Timed out",
			kind:    KindPlayerLeave,
			player:  "alice",
			reason:  "Timed out",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := Parse(
				"[12:34:56] [Server thread/INFO]: " + test.message,
			)
			assertKind(t, entry, test.kind)
			if entry.Player != test.player || entry.Reason != test.reason {
				t.Fatalf("Entry = %#v", entry)
			}
		})
	}
}

func TestParseModernLag(t *testing.T) {
	entry := Parse(
		"[12:34:56] [Server thread/WARN]: Can't keep up! " +
			"Is the server overloaded? Running 2531ms or 50 ticks behind",
	)

	assertLag(t, entry, 2531*time.Millisecond, 50)
}

func TestParseLegacyLag(t *testing.T) {
	entry := Parse(
		"[12:34:56] [Server thread/WARN]: Can't keep up! " +
			"Running 2531ms behind, skipping 50 tick(s)",
	)

	assertLag(t, entry, 2531*time.Millisecond, 50)
}

func TestParseLagWithoutDetails(t *testing.T) {
	entry := Parse(
		"[12:34:56] [Server thread/WARN]: Can't keep up! unknown format",
	)

	assertKind(t, entry, KindLag)
	if entry.Lag.BehindKnown || entry.Lag.TicksKnown {
		t.Fatalf("Lag = %#v", entry.Lag)
	}
}

func TestParseStripsANSIForClassification(t *testing.T) {
	entry := Parse(
		"\x1b[32m[12:34:56] [Server thread/INFO]: " +
			"<alice> green\x1b[0m",
	)

	assertKind(t, entry, KindChat)
	if entry.Chat != "green" {
		t.Fatalf("Entry = %#v", entry)
	}
}

func TestParseIgnoresJavaToolOptionsNotice(t *testing.T) {
	entry := Parse(
		"Picked up JAVA_TOOL_OPTIONS: -Xlog:gc:file=/tmp/hso/gc.log",
	)

	assertKind(t, entry, KindIgnored)
}

func TestParseKeepsOtherLines(t *testing.T) {
	line := "[12:34:56] [Server thread/INFO]: Done (1.234s)! For help, type \"help\""
	entry := Parse(line)

	assertKind(t, entry, KindOther)
	if entry.Raw != line ||
		entry.Message != "Done (1.234s)! For help, type \"help\"" {
		t.Fatalf("Entry = %#v", entry)
	}
}

func TestParseDoesNotTreatChatContentsAsLag(t *testing.T) {
	entry := Parse(
		"[12:34:56] [Server thread/INFO]: <alice> Can't keep up!",
	)

	assertKind(t, entry, KindChat)
}

func TestSentCommand(t *testing.T) {
	entry := SentCommand("say hello\n")

	assertKind(t, entry, KindCommand)
	if entry.Command != "say hello" || entry.Message != "say hello" {
		t.Fatalf("Entry = %#v", entry)
	}
}

func TestKindString(t *testing.T) {
	tests := map[Kind]string{
		KindOther:       "other",
		KindChat:        "chat",
		KindCommand:     "command",
		KindPlayerJoin:  "player_join",
		KindPlayerLeave: "player_leave",
		KindLag:         "lag",
		KindIgnored:     "ignored",
		Kind(255):       "other",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}

func assertKind(t *testing.T, entry Entry, want Kind) {
	t.Helper()
	if entry.Kind != want {
		t.Fatalf("Kind = %s, want %s; Entry = %#v", entry.Kind, want, entry)
	}
}

func assertLag(
	t *testing.T,
	entry Entry,
	wantBehind time.Duration,
	wantTicks uint64,
) {
	t.Helper()
	assertKind(t, entry, KindLag)
	if !entry.Lag.BehindKnown || entry.Lag.Behind != wantBehind {
		t.Fatalf("Lag.Behind = %#v, want %s", entry.Lag, wantBehind)
	}
	if !entry.Lag.TicksKnown || entry.Lag.TicksBehind != wantTicks {
		t.Fatalf("Lag.TicksBehind = %#v, want %d", entry.Lag, wantTicks)
	}
}
