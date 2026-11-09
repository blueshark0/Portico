package application

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	sessiondomain "portico/internal/sessions/domain"
)

type recentRepo struct {
	Repository
	items map[string][]sessiondomain.SessionBinding
}

func (r *recentRepo) GetRecentPlatformSessions(platform string, limit int) ([]sessiondomain.SessionBinding, error) {
	items := slices.Clone(r.items[platform])
	slices.SortFunc(items, func(a, b sessiondomain.SessionBinding) int {
		if c := b.LastSuccessAt.Compare(a.LastSuccessAt); c != 0 {
			return c
		}
		return strings.Compare(a.SessionID, b.SessionID)
	})
	return items[:min(limit, len(items))], nil
}

func TestListRecentSortsAcrossPlatformsAndBoundsResults(t *testing.T) {
	now := time.Now()
	repo := &recentRepo{items: map[string][]sessiondomain.SessionBinding{}}
	for i := range 70 {
		platform := []string{"claude", "codex", "gemini"}[i%3]
		repo.items[platform] = append(repo.items[platform], sessiondomain.SessionBinding{
			Platform: platform, SessionID: fmt.Sprintf("s%02d", i), LastSuccessAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	svc := NewService(repo)
	for _, tc := range []struct{ limit, want int }{{0, 5}, {-1, 5}, {3, 3}, {100, 50}} {
		items, err := svc.ListRecent(tc.limit)
		if err != nil || len(items) != tc.want || items[0].SessionID != "s69" {
			t.Fatalf("limit=%d: len=%d err=%v", tc.limit, len(items), err)
		}
	}
	repo.items = map[string][]sessiondomain.SessionBinding{
		"codex":  {{Platform: "codex", SessionID: "z", LastSuccessAt: now}},
		"claude": {{Platform: "claude", SessionID: "b", LastSuccessAt: now}, {Platform: "claude", SessionID: "a", LastSuccessAt: now}},
	}
	items, err := svc.ListRecent(5)
	if err != nil || items[0].SessionID != "a" || items[1].SessionID != "b" || items[2].Platform != "codex" {
		t.Fatalf("unstable tie ordering: %+v, %v", items, err)
	}
}
