package ui

import (
	"fmt"
	"math"
)

func (model *Model) statsTitle() string {
	title := fmt.Sprintf(
		"hijo-server-ops · %s · uptime %s",
		model.status,
		formatUptime(model.metrics.Uptime),
	)
	if model.jvmMetricError != "" || model.memoryMetricError != "" {
		title += " · metrics degraded"
	}
	return title
}

func (model *Model) statsLines() []string {
	postGC := "n/a"
	if model.gcStats.PostGC.Available {
		postGC = formatBytes(model.gcStats.PostGC.Value)
	}
	frequency := model.gcStats.Frequency()
	threads := "n/a"
	if model.metrics.Threads.Available {
		threads = fmt.Sprintf("%d", model.metrics.Threads.Value)
	}

	rows := []string{
		model.serverAddressLine(),
		fmt.Sprintf(
			"Heap %s / %s committed (max %s)  post-GC %s",
			formatJVMBytes(model.metrics.Heap.Used),
			formatJVMBytes(model.metrics.Heap.Committed),
			formatJVMBytes(model.metrics.Heap.Max),
			postGC,
		),
		model.rssLine(),
		fmt.Sprintf(
			"GC   %d collections  total %s  last %s  freq %s",
			model.gcStats.Collections,
			formatPause(
				model.gcStats.TotalPause,
				model.gcStats.LastPause.Available,
			),
			formatPause(
				model.gcStats.LastPause.Value,
				model.gcStats.LastPause.Available,
			),
			formatFrequency(frequency.PerMinute, frequency.Available),
		),
		fmt.Sprintf(
			"Players %d  Lag events: %d  CPU %s  threads %s",
			model.tracker.PlayerCount(),
			model.tracker.LagEvents(),
			highlight(
				formatCPU(model.cpu, model.cpuAvailable),
				cpuRatio(model.cpu, model.cpuAvailable),
			),
			threads,
		),
	}

	lines := make([]string, 0, len(rows)*2-2)
	for index, row := range rows {
		if index > 1 {
			lines = append(lines, "")
		}
		lines = append(lines, row)
	}
	return lines
}

func (model *Model) serverAddressLine() string {
	if model.serverIP == "" || model.serverPort == 0 {
		return "Server n/a"
	}
	return fmt.Sprintf("Server %s:%d", model.serverIP, model.serverPort)
}

func (model *Model) rssLine() string {
	rss := formatProcBytes(model.memory.RSS)
	ratio := highlight(formatRSSPercent(model.memory), rssRatio(model.memory))
	delta := formatDelta(model.memory.RSS, model.metrics.Heap.Committed)

	short := fmt.Sprintf("RSS  %s (%s)  Δ %s", rss, ratio, delta)
	limit, source := rssDenominator(model.memory)
	if source == "" {
		return short
	}

	label := "total"
	if source == "cgroup" {
		label = "limit"
	}
	full := fmt.Sprintf(
		"RSS  %s / %s %s (%s)  Δ %s",
		rss,
		formatBytes(limit),
		label,
		ratio,
		delta,
	)
	// 分母よりも診断上重要な RSS と heap committed の差分を残す。
	if stringWidth(full) > model.layout.statsWidth-2 {
		return short
	}
	return full
}

func (model *Model) renderGraphPanel() string {
	width := model.layout.leftContentWidth()
	var lines []string
	if model.jvmMetricError != "" {
		lines = append(lines, dimStyle.Render(truncate(
			"heap unavailable: "+model.jvmMetricError, width)))
	}
	if model.memoryMetricError != "" {
		lines = append(lines, dimStyle.Render(truncate(
			"rss unavailable: "+model.memoryMetricError, width)))
	}

	low, high, ok := model.graphRange()
	if ok {
		lines = append(lines, renderChart(
			model.samples,
			[]chartSeries{
				{value: heapValue, style: heapStyle},
				{value: rssValue, style: rssStyle},
			},
			width,
			model.layout.graphLines()-len(lines),
			low,
			high,
		)...)
	}
	return renderPanel(
		model.graphTitle(),
		lines,
		model.layout.leftWidth,
		model.layout.graphHeight,
		false,
		plainFrame,
	)
}

func (model *Model) graphTitle() string {
	title := "Graph · " +
		heapStyle.Render("heap") + dimStyle.Render(" / ") +
		rssStyle.Render("rss")
	if model.samples.Len() > 0 {
		title += fmt.Sprintf(" · %ds", model.samples.Len())
	}
	return title
}

func (model *Model) graphRange() (uint64, uint64, bool) {
	low := uint64(math.MaxUint64)
	high := uint64(0)
	for index := 0; index < model.samples.Len(); index++ {
		sample := model.samples.At(index)
		for _, value := range []sampleValue{heapValue, rssValue} {
			number, available := value(sample)
			if !available {
				continue
			}
			low = min(low, number)
			high = max(high, number)
		}
	}
	if high == 0 {
		return 0, 0, false
	}

	// 0 起点では小さな GC/RSS 変化が潰れるため実測範囲に余白を足す。
	margin := max((high-low)/20, high/100)
	if margin == 0 {
		margin = 1
	}
	return low - min(low, margin), high + margin, true
}

func (model *Model) renderMetersPanel() string {
	rss, source := rssMeter(model.memory)
	meters := []meter{
		cpuMeter(model.cpu, model.cpuAvailable),
		heapMeter(model.metrics.Heap),
		rss,
	}

	title := "Meters"
	if source != "" {
		title += " · RSS/" + source
	}
	return renderPanel(
		title,
		meterLines(meters, model.layout.metersContentWidth()),
		model.layout.metersWidth,
		statsHeight,
		false,
		plainFrame,
	)
}

func (model *Model) renderPlayersPanel() string {
	focused := model.mode == modeFocus && model.panel == panelPlayers
	viewport := model.layout.playerLines()
	width := model.layout.playersContentWidth()
	start := windowStart(model.playerCursor, len(model.playerList), viewport)

	lines := make([]string, viewport)
	for index := 0; index < viewport; index++ {
		position := start + index
		if position >= len(model.playerList) {
			continue
		}
		line := fitLine(truncate(model.playerList[position], width), width)
		if focused && position == model.playerCursor {
			line = selectedStyle.Render(line)
		}
		lines[index] = line
	}

	return renderPanel(
		fmt.Sprintf("%s %d", panelPlayers.title(), len(model.playerList)),
		lines,
		model.layout.playersWidth,
		statsHeight,
		false,
		model.frameFor(panelPlayers),
	)
}

func (model *Model) commandModal() (string, int, int) {
	width := stringWidth(model.playerTarget) + 5
	for _, command := range playerCommands {
		width = max(width, stringWidth(command.label)+4)
	}
	height := len(playerCommands) + 2

	lines := make([]string, 0, len(playerCommands))
	for index, command := range playerCommands {
		line := fitLine(" "+command.label, width-2)
		if index == model.commandCursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	start := windowStart(
		model.playerCursor,
		len(model.playerList),
		model.layout.playerLines(),
	)
	x := model.layout.statsWidth + model.layout.metersWidth
	y := model.playerCursor - start + 2
	x = clamp(x, 0, max(0, model.layout.width-width))
	y = clamp(y, 0, max(0, model.layout.height-height))

	box := renderPanel(model.playerTarget, lines, width, height, false, modalFrame)
	return box, x, y
}
