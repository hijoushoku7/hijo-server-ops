package javaenv

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Installation は自動検出した JVM を表す。
type Installation struct {
	Home        string
	Version     string
	Major       int
	Implementor string
	OSArch      string
}

type installationCandidate struct {
	installation Installation
	symlink      bool
}

// Installed は root 直下の JVM を列挙する。root は通常 /usr/lib/jvm を渡す。
func Installed(root string) ([]Installation, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	byRealPath := make(map[string]installationCandidate)
	for _, entry := range entries {
		name := entry.Name()
		if name == "default-java" || name == "default-java-runtime" {
			continue
		}
		home := filepath.Join(root, name)
		values, ok := readRelease(filepath.Join(home, "release"))
		if !ok {
			continue
		}
		major, ok := javaMajor(values["JAVA_VERSION"])
		if !ok {
			continue
		}
		if _, err := ValidateHome(home); err != nil {
			continue
		}
		real, err := filepath.EvalSymlinks(home)
		if err != nil {
			continue
		}
		candidate := Installation{Home: home, Version: values["JAVA_VERSION"], Major: major,
			Implementor: values["IMPLEMENTOR"], OSArch: values["OS_ARCH"]}
		candidateInfo := installationCandidate{installation: candidate, symlink: entry.Type()&os.ModeSymlink != 0}
		if current, exists := byRealPath[real]; !exists || prefer(candidateInfo, current) {
			byRealPath[real] = candidateInfo
		}
	}

	result := make([]Installation, 0, len(byRealPath))
	for _, candidate := range byRealPath {
		result = append(result, candidate.installation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Major != result[j].Major {
			return result[i].Major > result[j].Major
		}
		if comparison := compareVersions(result[i].Version, result[j].Version); comparison != 0 {
			return comparison > 0
		}
		return result[i].Home < result[j].Home
	})
	return result, nil
}

func compareVersions(left, right string) int {
	leftParts, rightParts := versionParts(left), versionParts(right)
	length := max(len(leftParts), len(rightParts))
	for i := 0; i < length; i++ {
		var leftPart, rightPart int
		if i < len(leftParts) {
			leftPart = leftParts[i]
		}
		if i < len(rightParts) {
			rightPart = rightParts[i]
		}
		if leftPart != rightPart {
			return leftPart - rightPart
		}
	}
	return strings.Compare(left, right)
}

func versionParts(version string) []int {
	var parts []int
	for _, field := range strings.FieldsFunc(version, func(r rune) bool { return !unicode.IsDigit(r) }) {
		part, err := strconv.Atoi(field)
		if err == nil {
			parts = append(parts, part)
		}
	}
	return parts
}

func readRelease(path string) (map[string]string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[strings.TrimSpace(key)] = value
	}
	return values, scanner.Err() == nil
}

func javaMajor(version string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) == 0 {
		return 0, false
	}
	part := parts[0]
	if part == "1" && len(parts) > 1 {
		part = parts[1]
	}
	end := strings.IndexFunc(part, func(r rune) bool { return !unicode.IsDigit(r) })
	if end >= 0 {
		part = part[:end]
	}
	major, err := strconv.Atoi(part)
	return major, err == nil && major > 0
}

func prefer(candidate, current installationCandidate) bool {
	if candidate.symlink != current.symlink {
		return candidate.symlink
	}
	candidateName := filepath.Base(candidate.installation.Home)
	currentName := filepath.Base(current.installation.Home)
	candidateVersioned := strings.IndexFunc(candidateName, unicode.IsDigit) >= 0
	currentVersioned := strings.IndexFunc(currentName, unicode.IsDigit) >= 0
	if candidateVersioned != currentVersioned {
		return candidateVersioned
	}
	if len(candidateName) != len(currentName) {
		return len(candidateName) < len(currentName)
	}
	return candidateName < currentName
}
