package hsperfdata

import (
	"fmt"
	"time"
)

type Number struct {
	Value     int64
	Available bool
}

type Duration struct {
	Value     time.Duration
	Available bool
}

type Memory struct {
	Used      Number
	Committed Number
	Max       Number
}

type Generation struct {
	Name   string
	Memory Memory
}

type Collector struct {
	Name        string
	Invocations Number
	Time        Duration
}

type Metrics struct {
	Heap                 Memory
	Generations          []Generation
	Metaspace            Memory
	CompressedClassSpace Memory
	Collectors           []Collector
	Uptime               Duration
	Threads              Number
}

func (s Snapshot) Metrics() Metrics {
	var metrics Metrics

	generationCount, _ := s.Long("sun.gc.generations")
	for index := int64(0); index < generationCount; index++ {
		prefix := fmt.Sprintf("sun.gc.generation.%d", index)
		name, _ := s.String(prefix + ".name")
		spaces, _ := s.Long(prefix + ".spaces")

		generation := Generation{
			Name: name,
			Memory: Memory{
				Committed: number(s, prefix+".capacity"),
				Max:       number(s, prefix+".maxCapacity"),
			},
		}
		for space := int64(0); space < spaces; space++ {
			used, ok := s.Long(fmt.Sprintf("%s.space.%d.used", prefix, space))
			if !ok {
				continue
			}
			generation.Memory.Used.Value += used
			generation.Memory.Used.Available = true
		}
		metrics.Generations = append(metrics.Generations, generation)
		addNumber(&metrics.Heap.Used, generation.Memory.Used)
		addNumber(&metrics.Heap.Committed, generation.Memory.Committed)
		addNumber(&metrics.Heap.Max, generation.Memory.Max)
	}
	if maximum, ok := s.Long("sun.gc.policy.maxCapacity"); ok {
		metrics.Heap.Max = Number{Value: maximum, Available: true}
	}

	metrics.Metaspace = memory(s, "sun.gc.metaspace")
	metrics.CompressedClassSpace = memory(s, "sun.gc.compressedclassspace")

	frequency, frequencyOK := s.Long("sun.os.hrt.frequency")
	if ticks, ok := s.Long("sun.os.hrt.ticks"); ok && frequencyOK && frequency > 0 {
		metrics.Uptime = Duration{
			Value:     ticksToDuration(ticks, frequency),
			Available: true,
		}
	}

	metrics.Threads = number(s, "java.threads.live")

	collectorCount, _ := s.Long("sun.gc.collectors")
	for index := int64(0); index < collectorCount; index++ {
		prefix := fmt.Sprintf("sun.gc.collector.%d", index)
		name, _ := s.String(prefix + ".name")
		collector := Collector{
			Name:        name,
			Invocations: number(s, prefix+".invocations"),
		}
		if ticks, ok := s.Long(prefix + ".time"); ok && frequencyOK && frequency > 0 {
			collector.Time = Duration{
				Value:     ticksToDuration(ticks, frequency),
				Available: true,
			}
		}
		metrics.Collectors = append(metrics.Collectors, collector)
	}

	return metrics
}

func memory(snapshot Snapshot, prefix string) Memory {
	return Memory{
		Used:      number(snapshot, prefix+".used"),
		Committed: number(snapshot, prefix+".capacity"),
		Max:       number(snapshot, prefix+".maxCapacity"),
	}
}

func number(snapshot Snapshot, name string) Number {
	value, ok := snapshot.Long(name)
	return Number{Value: value, Available: ok}
}

func addNumber(total *Number, value Number) {
	if !value.Available {
		return
	}
	total.Value += value.Value
	total.Available = true
}

func ticksToDuration(ticks, frequency int64) time.Duration {
	seconds := ticks / frequency
	remainder := ticks % frequency
	return time.Duration(seconds)*time.Second +
		time.Duration(remainder)*time.Second/time.Duration(frequency)
}
