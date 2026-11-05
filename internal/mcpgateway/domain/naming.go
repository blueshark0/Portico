package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// MaxPublicToolNameLen 网关侧公共工具名长度预算。
// CLI 侧还会叠加 mcp__<entry>__ 前缀（约 18 字符），Claude Code 排除全名
// 超 128 字符的工具，Anthropic API 工具名上限 64——按更严的 64 控制网关侧。
const MaxPublicToolNameLen = 64

// publicName 截断后留给哈希后缀的格式：full[:55] + "_" + sha256(full)[:8]。
const (
	truncatedBodyLen = 55
	hashSuffixLen    = 8
)

// SanitizeToolSegment 把上游工具名消毒为合法字符集 [A-Za-z0-9_-]：
// 非法字符替换为 '_'，空串兜底为 "x"。幂等。
func SanitizeToolSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

// PublicToolName 生成网关对外的工具名：<serverName>__<sanitizedTool>。
// serverName 经 Validate 保证只含 [A-Za-z0-9_-]，sanitize 后全名为纯 ASCII，
// 因此字节切片安全。超 MaxPublicToolNameLen 时确定性截断+哈希后缀。
func PublicToolName(serverName, upstreamTool string) string {
	full := serverName + "__" + SanitizeToolSegment(upstreamTool)
	if len(full) <= MaxPublicToolNameLen {
		return full
	}
	sum := sha256.Sum256([]byte(full))
	return full[:truncatedBodyLen] + "_" + hex.EncodeToString(sum[:])[:hashSuffixLen]
}
