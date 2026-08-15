package ui

import (
	"strings"
	"testing"
)

func TestStatsTitleIncludesServerNameAndVersion(t *testing.T) {
	model := New(
		make(chan Action, 1),
		nil,
		0,
		DefaultSettings(),
		ServerInfo{Name: "survival", Version: "v1.2.3"},
	)
	model.resize(160, minimumHeight)

	got := model.statsTitle()
	if !strings.Contains(got, "survival · hso v1.2.3 · starting") {
		t.Fatalf("title = %q", got)
	}
}

func TestStatsTitleFallsBackToProductName(t *testing.T) {
	model := New(
		make(chan Action, 1),
		nil,
		0,
		DefaultSettings(),
		ServerInfo{Version: "dev"},
	)
	model.resize(160, minimumHeight)

	if got := model.statsTitle(); !strings.HasPrefix(got, "hijo-server-ops · hso dev · starting") {
		t.Fatalf("title = %q", got)
	}
}

func TestStatsTitleDropsVersionOnNarrowTerminal(t *testing.T) {
	model := New(
		make(chan Action, 1),
		nil,
		0,
		DefaultSettings(),
		ServerInfo{Name: "survival", Version: "v1.2.3"},
	)
	model.resize(minimumWidth, minimumHeight)

	got := model.statsTitle()
	if strings.Contains(got, "hso v1.2.3") ||
		!strings.Contains(got, "survival") ||
		!strings.Contains(got, "starting") {
		t.Fatalf("title = %q", got)
	}
}

// TestStatsTitleTruncatesLongNameToKeepStatus は、登録名が長いだけで運転状況が
// 消えないことを見る。名前は 30 文字まで登録できるのに対し、最小幅の Stats の
// タイトルは 29 桁しかない。
func TestStatsTitleTruncatesLongNameToKeepStatus(t *testing.T) {
	model := New(
		make(chan Action, 1),
		nil,
		0,
		DefaultSettings(),
		ServerInfo{Name: strings.Repeat("a", 30), Version: "v1.2.3"},
	)
	model.resize(minimumWidth, minimumHeight)

	got := model.statsTitle()
	if !strings.Contains(got, "starting") || !strings.HasPrefix(got, "a") {
		t.Fatalf("title = %q", got)
	}
	if width := stringWidth(got); width > model.layout.statsWidth-5 {
		t.Fatalf("title = %q, width = %d, budget = %d",
			got, width, model.layout.statsWidth-5)
	}
}

func TestStatsTitleKeepsMetricsDegraded(t *testing.T) {
	model := New(
		make(chan Action, 1),
		nil,
		0,
		DefaultSettings(),
		ServerInfo{Name: "survival", Version: "dev"},
	)
	model.resize(160, minimumHeight)
	model.jvmMetricError = "unavailable"

	if got := model.statsTitle(); !strings.Contains(got, "metrics degraded") {
		t.Fatalf("title = %q", got)
	}
}

func TestWindowTitleIncludesServerName(t *testing.T) {
	model := New(
		make(chan Action, 1),
		nil,
		0,
		DefaultSettings(),
		ServerInfo{Name: "survival", Version: "dev"},
	)
	if got := model.View().WindowTitle; got != "survival · hijo-server-ops" {
		t.Fatalf("window title = %q", got)
	}

	model.info.Name = ""
	if got := model.View().WindowTitle; got != "hijo-server-ops" {
		t.Fatalf("fallback window title = %q", got)
	}
}
