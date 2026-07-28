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

	generationCount, generationsAvailable := s.Long("sun.gc.policy.generations")
	var (
		heapUsed      []Number
		heapCommitted []Number
		heapMax       []Number
	)
	for index := int64(0); index < generationCount; index++ {
		prefix := fmt.Sprintf("sun.gc.generation.%d", index)
		name, _ := s.String(prefix + ".name")
		spaces, spacesAvailable := s.Long(prefix + ".spaces")

		generation := Generation{
			Name: name,
			Memory: Memory{
				Committed: number(s, prefix+".capacity"),
				Max:       number(s, prefix+".maxCapacity"),
			},
		}
		if spacesAvailable {
			used := make([]Number, 0, spaces)
			for space := int64(0); space < spaces; space++ {
				used = append(used, number(
					s,
					fmt.Sprintf("%s.space.%d.used", prefix, space),
				))
			}
			generation.Memory.Used = sumNumbers(used)
		}
		metrics.Generations = append(metrics.Generations, generation)
		heapUsed = append(heapUsed, generation.Memory.Used)
		heapCommitted = append(heapCommitted, generation.Memory.Committed)
		heapMax = append(heapMax, generation.Memory.Max)
	}
	if generationsAvailable {
		metrics.Heap.Used = sumNumbers(heapUsed)
		metrics.Heap.Committed = sumNumbers(heapCommitted)
		metrics.Heap.Max = sumNumbers(heapMax)
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

	collectorCount, _ := s.Long("sun.gc.policy.collectors")
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

func sumNumbers(values []Number) Number {
	var total Number
	for _, value := range values {
		if !value.Available {
			return Number{}
		}
		total.Value += value.Value
	}
	total.Available = true
	return total
}

func ticksToDuration(ticks, frequency int64) time.Duration {
	seconds := ticks / frequency
	remainder := ticks % frequency
	return time.Duration(seconds)*time.Second +
		time.Duration(remainder)*time.Second/time.Duration(frequency)
}
