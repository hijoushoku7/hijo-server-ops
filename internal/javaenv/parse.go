package javaenv

import (
	"regexp"
	"strconv"
)

// ClassVersionError は UnsupportedClassVersionError から分かる Java の世代を表す。
type ClassVersionError struct {
	Required int
	Actual   int
}

var (
	requiredVersion = regexp.MustCompile(`class file version ([0-9]+)\.[0-9]+`)
	actualVersion   = regexp.MustCompile(`class file versions? up to ([0-9]+)\.[0-9]+`)
)

// ParseUnsupportedClassVersion はログを先頭から走査し、要求された Java と、
// 分かる場合は実際に起動した Java の世代を返す。
func ParseUnsupportedClassVersion(log string) (ClassVersionError, bool) {
	var result ClassVersionError
	location := requiredVersion.FindStringSubmatchIndex(log)
	if len(location) == 0 {
		return result, false
	}
	major, err := strconv.Atoi(log[location[2]:location[3]])
	if err != nil || major <= 44 {
		return result, false
	}
	result.Required = major - 44

	match := actualVersion.FindStringSubmatch(log[location[1]:])
	if len(match) != 0 {
		major, err = strconv.Atoi(match[1])
		if err == nil && major > 44 {
			result.Actual = major - 44
		}
	}
	return result, true
}
