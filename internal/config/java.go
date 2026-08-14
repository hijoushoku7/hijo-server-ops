package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hijoushoku7/hijo-server-ops/internal/javaenv"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

// SetJava は他の記述を保ったまま [server] の java だけを更新する。
func SetJava(path, javaHome string) error {
	before, err := Load(path)
	if err != nil {
		return err
	}
	javaHome, err = javaenv.ValidateHome(javaHome)
	if err != nil {
		return msg.JavaHomeInvalid(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return msg.ReadConfigFailed(err, path)
	}
	updated, err := replaceJava(data, javaHome)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".hso-java-*")
	if err != nil {
		return msg.WriteConfigFailed(err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			os.Remove(temporaryPath)
		}
	}()
	if _, err = temporary.Write(updated); err == nil {
		err = temporary.Chmod(permissionOf(path))
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return msg.WriteConfigFailed(err)
	}
	after, err := loadJavaUpdate(path, temporaryPath)
	if err != nil {
		return err
	}
	before.Server.Java = javaHome
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("localized Java update changed another config value")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return msg.ReplaceConfigFailed(err)
	}
	keepTemporary = false
	return nil
}

func loadJavaUpdate(path, temporaryPath string) (Config, error) {
	after, err := Load(temporaryPath)
	if err != nil {
		if cause := errors.Unwrap(err); cause != nil {
			err = cause
		}
		return Config{}, msg.ValidateJavaConfigFailed(err, path)
	}
	return after, nil
}

type tomlLine struct {
	start, contentEnd, end int
	assignmentEnd          int
	table                  []string
	key                    []string
	equal                  int
	header                 bool
}

func replaceJava(data []byte, javaHome string) ([]byte, error) {
	lines, newline := scanTOMLLines(data)
	serverStart, serverEnd := -1, len(data)
	// server = { ... } のインラインテーブルは書き換えの対象にしない。中身へ
	// 踏み込むと、この関数だけが持つ「ユーザーの設定を壊しうる」経路が増える。
	// hso が生成するのは常に [server] 形式なので、断って手で直してもらう。
	inlineServer := false
	for i := range lines {
		line := lines[i]
		if (len(line.table) == 0 && equalKey(line.key, "server", "java")) ||
			(equalKey(line.table, "server") && equalKey(line.key, "java")) {
			valueStart, valueEnd := assignmentValueBounds(data, line.equal+1, line.assignmentEnd)
			return splice(data, valueStart, valueEnd, quote(javaHome)), nil
		}
		if len(line.table) == 0 && equalKey(line.key, "server") {
			inlineServer = true
		}
		if line.header && equalKey(line.table, "server") {
			serverStart = line.end
			serverEnd = len(data)
			for j := i + 1; j < len(lines); j++ {
				if lines[j].header {
					serverEnd = lines[j].start
					break
				}
			}
		}
	}
	if serverStart < 0 && inlineServer {
		return nil, msg.JavaInlineTableUnsupported()
	}
	entry := "java = " + quote(javaHome)
	if serverStart >= 0 {
		prefix := ""
		if serverEnd > 0 && data[serverEnd-1] != '\n' && data[serverEnd-1] != '\r' {
			prefix = newline
		}
		suffix := newline
		if serverEnd == len(data) && !hasTrailingNewline(data) {
			suffix = ""
		}
		return splice(data, serverEnd, serverEnd, prefix+entry+suffix), nil
	}
	prefix := ""
	if len(data) > 0 && !hasTrailingNewline(data) {
		prefix = newline
	}
	suffix := ""
	if hasTrailingNewline(data) {
		suffix = newline
	}
	return append(append(append([]byte(nil), data...), []byte(prefix)...), []byte("server.java = "+quote(javaHome)+suffix)...), nil
}

func scanTOMLLines(data []byte) ([]tomlLine, string) {
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	var result []tomlLine
	table := []string(nil)
	multiline := byte(0)
	multilineAssignment := -1
	for start := 0; start < len(data); {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += start + 1
		}
		contentEnd := end
		if contentEnd > start && data[contentEnd-1] == '\n' {
			contentEnd--
		}
		if contentEnd > start && data[contentEnd-1] == '\r' {
			contentEnd--
		}
		line := tomlLine{start: start, contentEnd: contentEnd, end: end, assignmentEnd: contentEnd, table: append([]string(nil), table...), equal: -1}
		text := data[start:contentEnd]
		if multiline != 0 {
			if closesMultiline(text, multiline) {
				result[multilineAssignment].assignmentEnd = contentEnd
				multiline = 0
				multilineAssignment = -1
			}
		} else if parsed, ok := parseHeader(text); ok {
			table = parsed
			line.table = append([]string(nil), table...)
			line.header = true
		} else if key, equal, quote := parseAssignment(text); equal >= 0 {
			line.key, line.equal = key, start+equal
			multiline = quote
			if multiline != 0 {
				multilineAssignment = len(result)
			}
		}
		result = append(result, line)
		start = end
	}
	return result, newline
}

func parseHeader(line []byte) ([]string, bool) {
	s := strings.TrimSpace(string(line))
	if !strings.HasPrefix(s, "[") || strings.HasPrefix(s, "[[") {
		return nil, false
	}
	end := strings.IndexByte(s, ']')
	if end < 0 || strings.TrimSpace(s[end+1:]) != "" && !strings.HasPrefix(strings.TrimSpace(s[end+1:]), "#") {
		return nil, false
	}
	return parseKey(strings.TrimSpace(s[1:end]))
}

func parseAssignment(line []byte) ([]string, int, byte) {
	quote, escaped := byte(0), false
	for i, c := range line {
		if quote != 0 {
			if quote == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if c == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '#':
			return nil, -1, 0
		case '=':
			key, ok := parseKey(strings.TrimSpace(string(line[:i])))
			if !ok {
				return nil, -1, 0
			}
			value := bytes.TrimSpace(line[i+1:])
			multi := byte(0)
			if bytes.HasPrefix(value, []byte(`"""`)) && !closesMultiline(value[3:], '"') {
				multi = '"'
			} else if bytes.HasPrefix(value, []byte(`'''`)) && !closesMultiline(value[3:], '\'') {
				multi = '\''
			}
			return key, i, multi
		}
	}
	return nil, -1, 0
}

func parseKey(key string) ([]string, bool) {
	var parts []string
	for len(key) > 0 {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, false
		}
		var part string
		if key[0] == '\'' || key[0] == '"' {
			quote := key[0]
			end := 1
			for end < len(key) && key[end] != quote {
				if quote == '"' && key[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(key) {
				return nil, false
			}
			part = key[1:end]
			key = key[end+1:]
		} else {
			end := strings.IndexByte(key, '.')
			if end < 0 {
				part, key = strings.TrimSpace(key), ""
			} else {
				part, key = strings.TrimSpace(key[:end]), key[end:]
			}
		}
		parts = append(parts, part)
		key = strings.TrimSpace(key)
		if key == "" {
			return parts, true
		}
		if key[0] != '.' {
			return nil, false
		}
		key = key[1:]
	}
	return parts, len(parts) > 0
}

func closesMultiline(line []byte, quote byte) bool {
	needle := []byte{quote, quote, quote}
	return bytes.Contains(line, needle)
}

func assignmentValueBounds(data []byte, start, end int) (int, int) {
	for start < end && (data[start] == ' ' || data[start] == '\t') {
		start++
	}
	if start+3 <= end && (bytes.Equal(data[start:start+3], []byte(`"""`)) ||
		bytes.Equal(data[start:start+3], []byte(`'''`))) {
		delimiter := data[start : start+3]
		if closing := bytes.LastIndex(data[start+3:end], delimiter); closing >= 0 {
			return start, start + 3 + closing + 3
		}
	}
	quote, escaped := byte(0), false
	comment := end
	for i := start; i < end; i++ {
		c := data[i]
		if quote != 0 {
			if quote == '"' && c == '\\' && !escaped {
				escaped = true
				continue
			}
			if c == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		} else if c == '#' {
			comment = i
			break
		}
	}
	for comment > start && (data[comment-1] == ' ' || data[comment-1] == '\t') {
		comment--
	}
	return start, comment
}

func equalKey(key []string, want ...string) bool {
	return reflect.DeepEqual(key, want)
}

func splice(data []byte, start, end int, replacement string) []byte {
	result := make([]byte, 0, len(data)+len(replacement)-(end-start))
	result = append(result, data[:start]...)
	result = append(result, replacement...)
	return append(result, data[end:]...)
}

func hasTrailingNewline(data []byte) bool {
	return bytes.HasSuffix(data, []byte("\n")) || bytes.HasSuffix(data, []byte("\r"))
}
