package wails

import (
	"reflect"
	"testing"

	observabilitydomain "portico/internal/observability/domain"
	sessionapp "portico/internal/sessions/application"
	sessiondomain "portico/internal/sessions/domain"
)

type sessionListRepo struct {
	sessionapp.Repository
	bindings []sessiondomain.SessionBinding
}

func (r *sessionListRepo) GetPlatformSessions(string) ([]sessiondomain.SessionBinding, error) {
	return r.bindings, nil
}

type sessionUsageSpy struct {
	calls    int
	platform string
	ids      []string
}

func (q *sessionUsageSpy) SessionUsageForSessions(platform string, ids []string) (map[string]observabilitydomain.SessionUsage, error) {
	q.calls++
	q.platform, q.ids = platform, append([]string(nil), ids...)
	return map[string]observabilitydomain.SessionUsage{"active": {Requests: 3}}, nil
}

func TestSessionFacadeUsageIsLimitedToActiveBindings(t *testing.T) {
	repo := &sessionListRepo{bindings: []sessiondomain.SessionBinding{{Platform: "claude", SessionID: "active"}}}
	usage := &sessionUsageSpy{}
	facade := NewSessionFacade(sessionapp.NewService(repo), usage)
	summaries, err := facade.ListByPlatform("claude")
	if err != nil {
		t.Fatal(err)
	}
	if usage.calls != 1 || usage.platform != "claude" || !reflect.DeepEqual(usage.ids, []string{"active"}) {
		t.Fatalf("unexpected usage query: %+v", usage)
	}
	if len(summaries) != 1 || summaries[0].Requests != 3 {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}
	repo.bindings = nil
	summaries, err = facade.ListByPlatform("claude")
	if err != nil || len(summaries) != 0 || usage.calls != 1 {
		t.Fatalf("empty bindings must skip usage query: summaries=%+v calls=%d err=%v", summaries, usage.calls, err)
	}
}
