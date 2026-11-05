package infrastructure

import (
	"fmt"
	"strconv"
	"strings"
)

type envAssignment struct {
	key        string
	lineIndex  int
	valueStart int
	valueEnd   int
	value      string
	line       string
}

// readEnvFieldValues 保存目标 key 的完整原始赋值行（含原换行），因此重复
// key、export、引号、注释和原有顺序都可以在关闭时恢复。nil 表示原本不存在。
func readEnvFieldValues(data []byte, keys []string) (map[string][]string, error) {
	lines := splitEnvLines(string(data))
	values := make(map[string][]string, len(keys))
	for _, key := range keys {
		assignments, err := findEnvAssignments(lines, key)
		if err != nil {
			return nil, err
		}
		for _, assignment := range assignments {
			values[key] = append(values[key], assignment.line)
		}
	}
	return values, nil
}

func envEffectiveValues(data []byte, keys []string) (map[string]string, error) {
	lines := splitEnvLines(string(data))
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		assignments, err := findEnvAssignments(lines, key)
		if err != nil {
			return nil, err
		}
		if len(assignments) != 0 {
			// 与现有 parseEnvFile 的 map 覆盖语义一致：最后一个赋值生效。
			values[key] = assignments[len(assignments)-1].value
		}
	}
	return values, nil
}

// setEnvField 只替换已有赋值的 value 区间。注释、空行、export、键顺序和
// 其他内容保持原样；所有重复赋值一起更新，避免不同行残留不同有效值。
func setEnvField(data []byte, key, value string) ([]byte, error) {
	lines := splitEnvLines(string(data))
	assignments, err := findEnvAssignments(lines, key)
	if err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		return []byte(appendEnvLine(string(data), key+"="+value)), nil
	}
	for i := len(assignments) - 1; i >= 0; i-- {
		assignment := assignments[i]
		if assignment.value == value {
			continue
		}
		lineIndex := assignment.lineIndex
		line := lines[lineIndex]
		lines[lineIndex] = line[:assignment.valueStart] + value + line[assignment.valueEnd:]
	}
	return []byte(strings.Join(lines, "")), nil
}

// restoreEnvField 用接入前的原始赋值行替换当前目标 key。原本不存在的 key
// 删除其所有当前赋值；其他行及其相对顺序不变。
func restoreEnvField(data []byte, key string, originalLines []string) ([]byte, error) {
	for _, line := range originalLines {
		assignments, err := findEnvAssignments([]string{line}, key)
		if err != nil || len(assignments) != 1 {
			return nil, fmt.Errorf("dotenv 快照中 %s 的原始赋值无效", key)
		}
	}
	lines := splitEnvLines(string(data))
	assignments, err := findEnvAssignments(lines, key)
	if err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		if len(originalLines) == 0 {
			return data, nil
		}
		return []byte(appendEnvLines(string(data), originalLines)), nil
	}
	if len(assignments) == len(originalLines) {
		for i, assignment := range assignments {
			lines[assignment.lineIndex] = originalLines[i]
		}
		return []byte(strings.Join(lines, "")), nil
	}

	first := assignments[0].lineIndex
	remove := make(map[int]bool, len(assignments))
	for _, assignment := range assignments {
		remove[assignment.lineIndex] = true
	}
	result := make([]string, 0, len(lines)-len(assignments)+len(originalLines))
	hasFollowing := false
	for following := first + 1; following < len(lines); following++ {
		if !remove[following] {
			hasFollowing = true
			break
		}
	}
	for index, line := range lines {
		if index == first {
			inserted := append([]string(nil), originalLines...)
			if len(inserted) != 0 && hasFollowing {
				inserted = ensureEnvLinesTerminated(inserted, detectEnvNewline(string(data)))
			}
			result = append(result, inserted...)
		}
		if remove[index] {
			continue
		}
		result = append(result, line)
	}
	return []byte(strings.Join(result, "")), nil
}

func splitEnvLines(data string) []string {
	if data == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(data, "\n")+1)
	for len(data) != 0 {
		if index := strings.IndexByte(data, '\n'); index >= 0 {
			lines = append(lines, data[:index+1])
			data = data[index+1:]
			continue
		}
		lines = append(lines, data)
		break
	}
	return lines
}

func findEnvAssignments(lines []string, wantedKey string) ([]envAssignment, error) {
	var assignments []envAssignment
	for lineIndex, line := range lines {
		assignment, matched, err := parseEnvAssignment(line, lineIndex)
		if err != nil {
			if matched && assignment.key == wantedKey {
				return nil, err
			}
			continue
		}
		if matched && assignment.key == wantedKey {
			assignments = append(assignments, assignment)
		}
	}
	return assignments, nil
}

func parseEnvAssignment(line string, lineIndex int) (envAssignment, bool, error) {
	assignment := envAssignment{lineIndex: lineIndex, line: line}
	content := strings.TrimSuffix(line, "\n")
	content = strings.TrimSuffix(content, "\r")
	index := skipEnvSpace(content, 0)
	if index == len(content) || content[index] == '#' {
		return assignment, false, nil
	}
	if strings.HasPrefix(content[index:], "export") {
		end := index + len("export")
		if end < len(content) && isEnvSpace(content[end]) {
			index = skipEnvSpace(content, end)
		}
	}
	keyStart := index
	if index < len(content) && isEnvKeyStart(content[index]) {
		index++
		for index < len(content) && isEnvKeyPart(content[index]) {
			index++
		}
	}
	if keyStart == index {
		return assignment, false, nil
	}
	assignment.key = content[keyStart:index]
	index = skipEnvSpace(content, index)
	if index >= len(content) || content[index] != '=' {
		return assignment, false, nil
	}
	index++
	index = skipEnvSpace(content, index)
	assignment.valueStart = index
	if index < len(content) && (content[index] == '\'' || content[index] == '"') {
		quote := content[index]
		end := index + 1
		for end < len(content) {
			if content[end] == quote && (quote == '\'' || !envEscaped(content, end)) {
				break
			}
			end++
		}
		if end >= len(content) {
			return assignment, true, fmt.Errorf("dotenv 第 %d 行 %s 的引号未闭合", lineIndex+1, assignment.key)
		}
		rawQuoted := content[index : end+1]
		assignment.valueEnd = end + 1
		assignment.value = content[index+1 : end]
		if quote == '"' {
			if unquoted, err := strconv.Unquote(rawQuoted); err == nil {
				assignment.value = unquoted
			}
		}
		trailing := skipEnvSpace(content, end+1)
		if trailing < len(content) && content[trailing] != '#' {
			return assignment, true, fmt.Errorf("dotenv 第 %d 行 %s 的引号后存在无法识别的内容", lineIndex+1, assignment.key)
		}
		return assignment, true, nil
	}

	end := len(content)
	for index < len(content) {
		if content[index] == '#' && index > assignment.valueStart && isEnvSpace(content[index-1]) {
			end = index
			break
		}
		index++
	}
	for end > assignment.valueStart && isEnvSpace(content[end-1]) {
		end--
	}
	assignment.valueEnd = end
	assignment.value = content[assignment.valueStart:end]
	return assignment, true, nil
}

func appendEnvLine(data, line string) string {
	return appendEnvLines(data, []string{line})
}

func appendEnvLines(data string, lines []string) string {
	newline := detectEnvNewline(data)
	var builder strings.Builder
	builder.WriteString(data)
	if data != "" && !strings.HasSuffix(data, "\n") && !strings.HasSuffix(data, "\r") {
		builder.WriteString(newline)
	}
	for index, line := range lines {
		builder.WriteString(line)
		if !strings.HasSuffix(line, "\n") && !strings.HasSuffix(line, "\r") && index < len(lines)-1 {
			builder.WriteString(newline)
		}
	}
	return builder.String()
}

func ensureEnvLinesTerminated(lines []string, newline string) []string {
	if len(lines) == 0 {
		return lines
	}
	last := len(lines) - 1
	if !strings.HasSuffix(lines[last], "\n") && !strings.HasSuffix(lines[last], "\r") {
		lines[last] += newline
	}
	return lines
}

func detectEnvNewline(data string) string {
	if strings.Contains(data, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func skipEnvSpace(value string, index int) int {
	for index < len(value) && isEnvSpace(value[index]) {
		index++
	}
	return index
}

func isEnvSpace(char byte) bool { return char == ' ' || char == '\t' }
func isEnvKeyStart(char byte) bool {
	return char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}
func isEnvKeyPart(char byte) bool { return isEnvKeyStart(char) || char >= '0' && char <= '9' }
func envEscaped(value string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}
