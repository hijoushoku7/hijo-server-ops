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
