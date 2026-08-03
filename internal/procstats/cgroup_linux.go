package procstats

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type cgroupMembership struct {
	version int
	path    string
}

type cgroupMount struct {
	version    int
	root       string
	mountPoint string
}

func readCgroupMemory(mount cgroupMount, membership cgroupMembership) (Number, Limit) {
	directory := resolveCgroupDir(mount, membership.path)
	if membership.version == 2 {
		return readNumber(filepath.Join(directory, "memory.current")),
			readLimit(filepath.Join(directory, "memory.max"), false)
	}
	return readNumber(filepath.Join(directory, "memory.usage_in_bytes")),
		readLimit(filepath.Join(directory, "memory.limit_in_bytes"), true)
}

func readCgroups(path string) ([]cgroupMembership, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var memberships []cgroupMembership
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) != 3 || fields[2] == "" || !filepath.IsAbs(fields[2]) {
			continue
		}
		switch {
		case fields[0] == "0" && fields[1] == "":
			memberships = append(memberships, cgroupMembership{version: 2, path: fields[2]})
		case contains(fields[1], "memory"):
			memberships = append(memberships, cgroupMembership{version: 1, path: fields[2]})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, errors.New("not a member of a memory cgroup")
	}
	return memberships, nil
}

func readCgroupMounts(path string) ([]cgroupMount, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var mounts []cgroupMount
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := index(fields, "-")
		if separator < 6 || separator+3 >= len(fields) {
			continue
		}

		version := 0
		switch fields[separator+1] {
		case "cgroup2":
			version = 2
		case "cgroup":
			if contains(fields[separator+3], "memory") {
				version = 1
			}
		}
		if version == 0 {
			continue
		}
		mounts = append(mounts, cgroupMount{
			version:    version,
			root:       unescapeMountField(fields[3]),
			mountPoint: unescapeMountField(fields[4]),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(mounts) == 0 {
		return nil, errors.New("no memory cgroup mount")
	}
	return mounts, nil
}

func resolveCgroupDir(mount cgroupMount, group string) string {
	root := filepath.Clean(mount.root)
	group = filepath.Clean(group)
	relative := strings.TrimPrefix(group, string(filepath.Separator))
	if root != string(filepath.Separator) {
		switch {
		case group == root:
			relative = ""
		case strings.HasPrefix(group, root+string(filepath.Separator)):
			relative = strings.TrimPrefix(group, root+string(filepath.Separator))
		}
	}
	return filepath.Join(mount.mountPoint, relative)
}

func readNumber(path string) Number {
	data, err := os.ReadFile(path)
	if err != nil {
		return Number{}
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return Number{}
	}
	return Number{Value: value, Available: true}
}

func readLimit(path string, version1 bool) Limit {
	data, err := os.ReadFile(path)
	if err != nil {
		return Limit{}
	}
	value := strings.TrimSpace(string(data))
	if value == "max" {
		return Limit{Available: true, Unlimited: true}
	}
	limit, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return Limit{}
	}
	const cgroupV1Unlimited = uint64(1<<63 - 4096)
	if version1 && limit >= cgroupV1Unlimited {
		return Limit{Available: true, Unlimited: true}
	}
	return Limit{Value: limit, Available: true}
}

func contains(list, item string) bool {
	for _, value := range strings.Split(list, ",") {
		if value == item {
			return true
		}
	}
	return false
}

func index(values []string, target string) int {
	for position, value := range values {
		if value == target {
			return position
		}
	}
	return -1
}

func unescapeMountField(value string) string {
	return strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	).Replace(value)
}
