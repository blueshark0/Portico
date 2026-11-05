package relay

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	routingdomain "portico/internal/routing/domain"
)

func applyRequestBodyReplacements(
	bodyBytes []byte,
	replacements []routingdomain.RequestBodyReplacement,
) ([]byte, error) {
	current := bodyBytes
	for _, replacement := range replacements {
		path := strings.TrimSpace(replacement.Path)
		if path == "" {
			continue
		}
		target := gjson.GetBytes(current, path)
		if !target.Exists() {
			continue
		}
		rawValue := []byte(strings.TrimSpace(replacement.Value))
		if !json.Valid(rawValue) {
			return bodyBytes, fmt.Errorf("请求体替换值不是合法 JSON: path=%s", path)
		}
		// MatchValue 非空表示条件替换：当前值与匹配值语义相等才改写；为空表示覆写。
		if matchValue := strings.TrimSpace(replacement.MatchValue); matchValue != "" {
			if !json.Valid([]byte(matchValue)) {
				return bodyBytes, fmt.Errorf("请求体匹配值不是合法 JSON: path=%s", path)
			}
			if !jsonLiteralEqual(target.Raw, matchValue) {
				continue
			}
		}
		modified, err := sjson.SetRawBytes(current, path, rawValue)
		if err != nil {
			return bodyBytes, fmt.Errorf("替换请求体字段失败: path=%s: %w", path, err)
		}
		current = modified
	}
	return current, nil
}

// jsonLiteralEqual 按语义比较两个 JSON 字面量：解析后深度相等（0.20 等于 0.2、
// 忽略对象空格差异）；任一侧解析失败时退回原始字符串比较。
func jsonLiteralEqual(raw, literal string) bool {
	var parsedRaw, parsedLiteral interface{}
	rawErr := json.Unmarshal([]byte(raw), &parsedRaw)
	literalErr := json.Unmarshal([]byte(literal), &parsedLiteral)
	if rawErr != nil || literalErr != nil {
		return raw == literal
	}
	return reflect.DeepEqual(parsedRaw, parsedLiteral)
}
