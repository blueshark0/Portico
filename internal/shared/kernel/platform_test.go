package kernel

import (
	"testing"
	"time"
)

func TestParsePlatform(t *testing.T) {
	for _, valid := range []string{"claude", "codex", "gemini"} {
		platform, err := ParsePlatform(valid)
		if err != nil {
			t.Fatalf("ParsePlatform(%q): %v", valid, err)
		}
		if platform.String() != valid {
			t.Fatalf("ParsePlatform(%q) = %q", valid, platform)
		}
	}
	if _, err := ParsePlatform("openai"); err == nil {
		t.Fatal("expected unknown platform to be rejected")
	}
}

func TestSessionTimeoutPerPlatform(t *testing.T) {
	if got := PlatformClaude.SessionTimeout(); got != 5*time.Minute {
		t.Fatalf("claude timeout = %v", got)
	}
	if got := PlatformCodex.SessionTimeout(); got != 15*time.Minute {
		t.Fatalf("codex timeout = %v", got)
	}
	if got := PlatformGemini.SessionTimeout(); got != 60*time.Minute {
		t.Fatalf("gemini timeout = %v", got)
	}
}
