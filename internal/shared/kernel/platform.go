package kernel

import (
	"fmt"
	"time"
)

type Platform string

const (
	PlatformClaude Platform = "claude"
	PlatformCodex  Platform = "codex"
	PlatformGemini Platform = "gemini"
)

func AllPlatforms() []Platform {
	return []Platform{PlatformClaude, PlatformCodex, PlatformGemini}
}

func ParsePlatform(value string) (Platform, error) {
	switch Platform(value) {
	case PlatformClaude, PlatformCodex, PlatformGemini:
		return Platform(value), nil
	default:
		return "", fmt.Errorf("unknown platform: %s", value)
	}
}

func MustPlatform(value string) Platform {
	platform, err := ParsePlatform(value)
	if err != nil {
		panic(err)
	}
	return platform
}

func (p Platform) String() string {
	return string(p)
}

func (p Platform) SessionTimeout() time.Duration {
	switch p {
	case PlatformCodex:
		return 15 * time.Minute
	case PlatformGemini:
		return 60 * time.Minute
	default:
		return 5 * time.Minute
	}
}
