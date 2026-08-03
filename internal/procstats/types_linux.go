package procstats

import "time"

type Number struct {
	Value     uint64
	Available bool
}

type Limit struct {
	Value     uint64
	Available bool
	Unlimited bool
}

type Memory struct {
	RSS           Number
	CgroupCurrent Number
	CgroupLimit   Limit
	// cgroup 制限がない環境でも RSS の割合を出せるよう保持する。
	HostTotal Number
}

type Duration struct {
	Value     time.Duration
	Available bool
}
