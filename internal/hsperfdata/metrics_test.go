package hsperfdata

import (
	"testing"
	"time"
)

func TestMetrics(t *testing.T) {
	snapshot := Snapshot{Counters: map[string]Counter{
		"sun.gc.generations":                      {long: 2},
		"sun.gc.generation.0.name":                {text: "young", isString: true},
		"sun.gc.generation.0.spaces":              {long: 2},
		"sun.gc.generation.0.capacity":            {long: 300},
		"sun.gc.generation.0.maxCapacity":         {long: 600},
		"sun.gc.generation.0.space.0.used":        {long: 100},
		"sun.gc.generation.0.space.1.used":        {long: 20},
		"sun.gc.generation.1.name":                {text: "old", isString: true},
		"sun.gc.generation.1.spaces":              {long: 1},
		"sun.gc.generation.1.capacity":            {long: 700},
		"sun.gc.generation.1.maxCapacity":         {long: 1400},
		"sun.gc.generation.1.space.0.used":        {long: 400},
		"sun.gc.policy.maxCapacity":               {long: 2048},
		"sun.gc.metaspace.used":                   {long: 50},
		"sun.gc.metaspace.capacity":               {long: 80},
		"sun.gc.metaspace.maxCapacity":            {long: 500},
		"sun.gc.compressedclassspace.used":        {long: 10},
		"sun.gc.compressedclassspace.capacity":    {long: 20},
		"sun.gc.compressedclassspace.maxCapacity": {long: 100},
		"sun.gc.collectors":                       {long: 1},
		"sun.gc.collector.0.name":                 {text: "young GC", isString: true},
		"sun.gc.collector.0.invocations":          {long: 12},
		"sun.gc.collector.0.time":                 {long: 1250},
		"sun.os.hrt.frequency":                    {long: 1000},
		"sun.os.hrt.ticks":                        {long: 12_500},
		"java.threads.live":                       {long: 37},
	}}

	metrics := snapshot.Metrics()

	assertNumber(t, metrics.Heap.Used, 520)
	assertNumber(t, metrics.Heap.Committed, 1000)
	assertNumber(t, metrics.Heap.Max, 2048)
	assertNumber(t, metrics.Metaspace.Used, 50)
	assertNumber(t, metrics.CompressedClassSpace.Max, 100)
	assertNumber(t, metrics.Threads, 37)
	if len(metrics.Generations) != 2 || metrics.Generations[1].Name != "old" {
		t.Fatalf("Generations = %#v", metrics.Generations)
	}
	if !metrics.Uptime.Available || metrics.Uptime.Value != 12_500*time.Millisecond {
		t.Fatalf("Uptime = %#v", metrics.Uptime)
	}
	if len(metrics.Collectors) != 1 {
		t.Fatalf("Collectors = %#v", metrics.Collectors)
	}
	if metrics.Collectors[0].Name != "young GC" {
		t.Fatalf("collector name = %q", metrics.Collectors[0].Name)
	}
	assertNumber(t, metrics.Collectors[0].Invocations, 12)
	if !metrics.Collectors[0].Time.Available ||
		metrics.Collectors[0].Time.Value != 1250*time.Millisecond {
		t.Fatalf("collector time = %#v", metrics.Collectors[0].Time)
	}
}

func TestMetricsKeepsUnavailableValuesExplicit(t *testing.T) {
	metrics := (Snapshot{Counters: map[string]Counter{}}).Metrics()

	if metrics.Heap.Used.Available ||
		metrics.Heap.Committed.Available ||
		metrics.Heap.Max.Available ||
		metrics.Uptime.Available ||
		metrics.Threads.Available {
		t.Fatalf("Metrics = %#v", metrics)
	}
}

func assertNumber(t *testing.T, got Number, want int64) {
	t.Helper()
	if !got.Available || got.Value != want {
		t.Fatalf("number = %#v, want %d", got, want)
	}
}
