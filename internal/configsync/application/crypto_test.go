package application

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	configsyncdomain "portico/internal/configsync/domain"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func sampleSnapshot() configsyncdomain.ConfigSnapshot {
	defaultID := 1
	return configsyncdomain.ConfigSnapshot{
		Backend: configsyncdomain.BackendSnapshot{
			AppPreferences: routingdomain.AppPreferences{
				ShowHeatmap:   true,
				ShowHomeTitle: true,
				ProxyEnabled:  true,
				ProxyURL:      "http://127.0.0.1:7890",
			},
			ClaudeProfile: routingdomain.RouteProfile{
				Platform:          kernel.PlatformClaude,
				DefaultProviderID: &defaultID,
				Providers: []routingdomain.Provider{{
					ID:              1,
					Name:            "alpha",
					APIURL:          "https://api.example.com",
					APIKey:          "sk-alpha",
					OfficialSite:    "https://example.com",
					Tint:            "rgba(0,0,0,0.1)",
					Accent:          "#0a84ff",
					Position:        1,
					SupportedModels: map[string]bool{"claude-sonnet-4": true},
					ModelMapping:    map[string]string{"claude-*": "vendor/claude-*"},
					RequestBodyReplacements: []routingdomain.RequestBodyReplacement{
						{Path: "metadata.user_id", Value: "fixed"},
					},
					ProxyMode:          "custom",
					ProxyURL:           "socks5://127.0.0.1:1080",
					InsecureSkipVerify: true,
				}},
			},
			CodexProfile:  routingdomain.RouteProfile{Platform: kernel.PlatformCodex, Providers: []routingdomain.Provider{}},
			GeminiProfile: routingdomain.RouteProfile{Platform: kernel.PlatformGemini, Providers: []routingdomain.Provider{}},
		},
		UI: configsyncdomain.UIPreferences{Theme: "dark", Locale: "zh"},
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	snapshot := sampleSnapshot()
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	payload, meta, err := EncryptSnapshot(snapshot, "correct horse", "device-a", "1.0.0", now)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if meta.UpdatedAt != "2026-07-07T10:00:00Z" || meta.DeviceName != "device-a" || meta.SchemaVersion != configsyncdomain.SchemaVersion {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	decrypted, decMeta, err := DecryptSnapshot(payload, "correct horse")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decMeta.UpdatedAt != meta.UpdatedAt || decMeta.AppVersion != "1.0.0" {
		t.Fatalf("unexpected decrypted meta: %+v", decMeta)
	}

	original, _ := json.Marshal(snapshot)
	restored, _ := json.Marshal(decrypted)
	if string(original) != string(restored) {
		t.Fatalf("round-trip mismatch:\n original: %s\n restored: %s", original, restored)
	}
}

func TestDecryptSnapshotRejectsWrongPassphrase(t *testing.T) {
	payload, _, err := EncryptSnapshot(sampleSnapshot(), "correct horse", "device-a", "1.0.0", time.Now())
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, _, err := DecryptSnapshot(payload, "wrong passphrase"); err == nil {
		t.Fatal("expected wrong passphrase to fail decryption")
	}
}

func TestDecryptSnapshotRejectsTamperedCiphertext(t *testing.T) {
	payload, _, err := EncryptSnapshot(sampleSnapshot(), "correct horse", "device-a", "1.0.0", time.Now())
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(envelope["ciphertext"].(string))
	raw[0] ^= 0xff
	envelope["ciphertext"] = base64.StdEncoding.EncodeToString(raw)
	tampered, _ := json.Marshal(envelope)

	if _, _, err := DecryptSnapshot(tampered, "correct horse"); err == nil {
		t.Fatal("expected tampered ciphertext to fail authentication")
	}
}

func TestDecryptSnapshotRejectsGarbage(t *testing.T) {
	if _, _, err := DecryptSnapshot([]byte("not json at all"), "pass"); err == nil {
		t.Fatal("expected garbage payload to be rejected")
	}
	if _, _, err := DecryptSnapshot([]byte(`{"schemaVersion":0}`), "pass"); err == nil {
		t.Fatal("expected incomplete envelope to be rejected")
	}
}

func TestEncryptSnapshotRequiresPassphrase(t *testing.T) {
	if _, _, err := EncryptSnapshot(sampleSnapshot(), "   ", "device", "1.0.0", time.Now()); err == nil {
		t.Fatal("expected blank passphrase to be rejected")
	}
}

func TestReadEnvelopeMeta(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	payload, _, err := EncryptSnapshot(sampleSnapshot(), "pass", "laptop", "0.9.9", now)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	meta, err := ReadEnvelopeMeta(payload)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if !meta.Exists || meta.DeviceName != "laptop" || meta.AppVersion != "0.9.9" || meta.UpdatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}

// TestSnapshotV1WireFormatIsStable 锁定云端 v1 快照的序列化契约：
// BackendSnapshot 直接内嵌 routingdomain 结构体，routing 模型的任何字段
// 或 json tag 调整都会改变跨设备快照格式——此测试失败说明改动破坏了
// v1 兼容性，需要提供迁移或恢复 tag。
func TestSnapshotV1WireFormatIsStable(t *testing.T) {
	got, err := json.Marshal(sampleSnapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	want := strings.Join([]string{
		`{"backend":{"appPreferences":{"show_heatmap":true,"show_home_title":true,"proxy_enabled":true,"proxy_url":"http://127.0.0.1:7890"},`,
		`"claudeProfile":{"platform":"claude","defaultProviderId":1,"providers":[`,
		`{"id":1,"name":"alpha","apiUrl":"https://api.example.com","apiKey":"sk-alpha","officialSite":"https://example.com",`,
		`"tint":"rgba(0,0,0,0.1)","accent":"#0a84ff","position":1,`,
		`"supportedModels":{"claude-sonnet-4":true},"modelMapping":{"claude-*":"vendor/claude-*"},`,
		`"requestBodyReplacements":[{"path":"metadata.user_id","value":"fixed"}],`,
		`"proxyMode":"custom","proxyUrl":"socks5://127.0.0.1:1080","insecureSkipVerify":true}]},`,
		`"codexProfile":{"platform":"codex","defaultProviderId":null,"providers":[]},`,
		`"geminiProfile":{"platform":"gemini","defaultProviderId":null,"providers":[]}},`,
		`"ui":{"theme":"dark","locale":"zh"}}`,
	}, "")
	if string(got) != want {
		t.Fatalf("v1 snapshot wire format changed:\n got: %s\nwant: %s", got, want)
	}
}
