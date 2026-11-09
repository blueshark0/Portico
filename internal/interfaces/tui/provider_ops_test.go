package tui

import (
	"strings"
	"testing"

	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func intPtr(v int) *int { return &v }

func sampleProfile() routingdomain.RouteProfile {
	return routingdomain.RouteProfile{
		Platform:          kernel.PlatformClaude,
		DefaultProviderID: intPtr(1),
		Providers: []routingdomain.Provider{
			{ID: 1, Name: "alpha", APIURL: "https://a.example.com", APIKey: "ka", Position: 1,
				ModelMapping: map[string]string{"sonnet": "glm-4"}},
			{ID: 3, Name: "beta", APIURL: "https://b.example.com", Position: 2},
			{ID: 4, Name: "gamma", APIURL: "https://c.example.com", Position: 3, Archived: true},
		},
	}
}

func TestSetDefaultProvider(t *testing.T) {
	profile := sampleProfile()
	next, err := setDefaultProvider(profile, 3)
	if err != nil {
		t.Fatal(err)
	}
	if *next.DefaultProviderID != 3 || *profile.DefaultProviderID != 1 {
		t.Fatalf("default = %d, original = %d", *next.DefaultProviderID, *profile.DefaultProviderID)
	}
	if _, err := setDefaultProvider(profile, 4); err == nil {
		t.Fatal("archived provider must not become default")
	}
	if _, err := setDefaultProvider(profile, 99); err == nil {
		t.Fatal("unknown provider must fail")
	}
}

func TestUpsertProviderCreatesAtTopWithNextID(t *testing.T) {
	profile := sampleProfile()
	next, err := upsertProvider(profile, 0, providerInput{Name: " delta ", APIURL: "https://d.example.com/ ", APIKey: " kd "})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Providers) != 4 || len(profile.Providers) != 3 {
		t.Fatalf("providers = %d, original = %d", len(next.Providers), len(profile.Providers))
	}
	created := next.Providers[0]
	if created.ID != 5 || created.Name != "delta" || created.APIURL != "https://d.example.com/" || created.APIKey != "kd" {
		t.Fatalf("created = %+v", created)
	}
	for i, provider := range next.Providers {
		if provider.Position != i+1 {
			t.Fatalf("position of %s = %d, want %d", provider.Name, provider.Position, i+1)
		}
	}
}

func TestUpsertProviderEditKeepsOtherFields(t *testing.T) {
	profile := sampleProfile()
	next, err := upsertProvider(profile, 1, providerInput{Name: "renamed", APIURL: "https://new.example.com", APIKey: "new"})
	if err != nil {
		t.Fatal(err)
	}
	edited := next.Providers[0]
	if edited.Name != "alpha" {
		t.Fatalf("existing provider name must not change, got %q", edited.Name)
	}
	if edited.APIURL != "https://new.example.com" || edited.APIKey != "new" || edited.ModelMapping["sonnet"] != "glm-4" {
		t.Fatalf("edited = %+v", edited)
	}
	if profile.Providers[0].APIKey != "ka" {
		t.Fatal("original profile must not be mutated")
	}
}

func TestUpsertProviderValidation(t *testing.T) {
	profile := sampleProfile()
	cases := map[string]providerInput{
		"名称不能为空": {Name: " ", APIURL: "https://x.example.com"},
		"已存在":    {Name: "BETA", APIURL: "https://x.example.com"},
		"API 地址": {Name: "new", APIURL: "ftp://x.example.com"},
	}
	for want, input := range cases {
		if _, err := upsertProvider(profile, 0, input); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("input %+v: err = %v, want containing %q", input, err, want)
		}
	}
	// 编辑自身时同名不算重复
	if _, err := upsertProvider(profile, 3, providerInput{Name: "beta", APIURL: "https://b.example.com"}); err != nil {
		t.Fatalf("editing self: %v", err)
	}
}

func TestArchiveAndDeleteGuardDefault(t *testing.T) {
	profile := sampleProfile()
	if _, err := setProviderArchived(profile, 1, true); err == nil {
		t.Fatal("default provider must not be archived")
	}
	if _, err := deleteProvider(profile, 1); err == nil {
		t.Fatal("default provider must not be deleted")
	}
	restored, err := setProviderArchived(profile, 4, false)
	if err != nil || restored.Providers[2].Archived {
		t.Fatalf("restore: %v %+v", err, restored.Providers[2])
	}
	next, err := deleteProvider(profile, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Providers) != 2 || next.Providers[1].Name != "gamma" || next.Providers[1].Position != 2 {
		t.Fatalf("after delete = %+v", next.Providers)
	}
	if len(profile.Providers) != 3 || profile.Providers[1].Name != "beta" {
		t.Fatal("original profile must not be mutated by delete")
	}
}
