package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func (model *Model) updateMetrics(message MetricsMsg) {
	model.metrics = hsperfdata.Metrics{
		Heap:    message.JVM.Heap,
		Uptime:  message.JVM.Uptime,
		Threads: message.JVM.Threads,
	}
	model.memory = message.Memory
	model.cpu = message.CPU
	model.cpuAvailable = message.CPUAvailable
	model.updateMetricError("heap", &model.jvmMetricError, message.JVMError)
	model.updateMetricError("RSS", &model.memoryMetricError, message.MemoryError)
	model.rememberLastMetrics(message)
	model.addSample(message)
}

// rememberLastMetrics は取れた値だけを控える。プロセスが消えてから終了を
// 検知するまでの隙間に走った採取は全部 n/a で返るので、そのまま持つと
// 終了モーダルに出す最終メモリが消える。ダッシュボードの表示は生の値の
// ままにして、控えはモーダル専用にする。
func (model *Model) rememberLastMetrics(message MetricsMsg) {
	// 項目ごとに控える。hsperfdata は一部のカウンタだけ読めないことがあり、
	// まとめて置き換えると取れていた committed が取れない値で消える。
	rememberNumber(&model.lastHeap.Used, message.JVM.Heap.Used)
	rememberNumber(&model.lastHeap.Committed, message.JVM.Heap.Committed)
	rememberNumber(&model.lastHeap.Max, message.JVM.Heap.Max)
	if message.Memory.RSS.Available {
		model.lastRSS = message.Memory.RSS
	}
}

func rememberNumber(last *hsperfdata.Number, next hsperfdata.Number) {
	if next.Available {
		*last = next
	}
}

func (model *Model) updateServerAddress(message ServerAddressMsg) {
	model.serverIP = message.IP
	model.serverPort = message.Port
	if message.IPErr != "" {
		model.serverIP = ""
		model.logUnavailableAddress("public IPv4", message.IPErr)
	}
	if message.PortErr != "" {
		model.serverPort = 0
		model.logUnavailableAddress("server-port", message.PortErr)
	}
}

// copyServerAddress は c キーで呼ばれる。アドレスが取れていないときは
// クリップボードを書き換えず、無反応のままにする。
func (model *Model) copyServerAddress() tea.Cmd {
	address, ok := model.serverAddress()
	if !ok {
		return nil
	}
	model.addLog(serverlog.Entry{
		Kind:    serverlog.KindOther,
		Message: msg.AddressCopied(address),
	})
	return tea.SetClipboard(address)
}

func (model *Model) logUnavailableAddress(source, detail string) {
	model.addLog(serverlog.Entry{
		Kind: serverlog.KindOther,
		Message: "server address: " + source + " unavailable: " +
			truncateRunes(detail, maxMetricErrorRunes),
	})
}

func (model *Model) resetServerState() {
	model.metrics = hsperfdata.Metrics{}
	model.memory = procstats.Memory{}
	model.cpu = 0
	model.cpuAvailable = false
	model.jvmMetricError = ""
	model.memoryMetricError = ""
	model.serverIP = ""
	model.serverPort = 0
	model.lastHeap = hsperfdata.Memory{}
	model.lastRSS = procstats.Number{}
	model.gcStats = gclog.Stats{}
	model.tracker = serverlog.Tracker{}
	model.playerList = nil
	model.completionOpen = false
	model.completionCursor = 0
	model.playerCursor = 0
	model.playerStage = playerStagePlayers
	model.samples = sampleBuffer{}
	model.samples.SetLimit(model.layout.graphWidth * 2)
}

func (model *Model) resize(width, height int) {
	model.layout = calculateLayout(width, height)
	// 履歴は表示可否や表示行数と切り離し、常に一定量を保持する。
	model.chat.SetLimit(historyLines)
	model.logs.SetLimit(historyLines)
	model.samples.SetLimit(model.layout.graphWidth * 2)
}

func (model *Model) addLog(entry serverlog.Entry) {
	if entry.Kind == serverlog.KindIgnored {
		return
	}
	model.tracker.Apply(entry)
	if entry.Kind == serverlog.KindPlayerJoin ||
		entry.Kind == serverlog.KindPlayerLeave {
		model.playerList = model.tracker.Players()
		model.playerCursor = clamp(
			model.playerCursor,
			0,
			max(0, len(model.playerList)-1),
		)
		if model.completionOpen {
			model.refreshCompletions()
		}
	}

	switch entry.Kind {
	case serverlog.KindChat:
		model.chat.Add(newLogRecord(entry, entry.Chat))
	case serverlog.KindCommand:
		record := newLogRecord(entry, entry.Command)
		model.logs.Add(record)
		if entry.SentByHSO {
			// サーバーログ由来のささやきはバージョンや MOD で形式が変わるため
			// 分類せず、SentCommand が印を付けた hso 自身の送信だけを扱う。
			if target, body, ok := serverlog.Whisper(entry.Command); ok {
				record.kind = serverlog.KindChat
				record.player = "→ " + target
				record.text = body
				model.chat.Add(record)
			}
		}
	default:
		model.logs.Add(newLogRecord(entry, entry.Message))
	}
}

func newLogRecord(entry serverlog.Entry, text string) logRecord {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
		entry.TimestampSource = serverlog.TimestampReceived
	}
	return logRecord{
		timestamp:       entry.Timestamp,
		timestampSource: entry.TimestampSource,
		kind:            entry.Kind,
		player:          entry.Player,
		text:            text,
	}
}

func (model *Model) addSample(message MetricsMsg) {
	sample := memorySample{}
	if message.JVM.Heap.Used.Available && message.JVM.Heap.Used.Value >= 0 {
		sample.heap = uint64(message.JVM.Heap.Used.Value)
		sample.heapKnown = true
	}
	if message.Memory.RSS.Available {
		sample.rss = message.Memory.RSS.Value
		sample.rssKnown = true
	}
	model.samples.Add(sample)
}

func (model *Model) updateMetricError(source string, current *string, next string) {
	if next == *current {
		return
	}
	next = truncateRunes(next, maxMetricErrorRunes)
	if next == *current {
		return
	}
	if next == "" {
		if *current != "" {
			model.addLog(serverlog.Entry{
				Kind:    serverlog.KindOther,
				Message: "metrics: " + source + " recovered",
			})
		}
	} else {
		model.addLog(serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: "metrics: " + source + " unavailable: " + next,
		})
	}
	*current = next
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
