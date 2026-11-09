package application

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"portico/internal/mcpgateway/domain"
)

func TestRuntimeSnapshotTracksContentAndOmitsUnchangedCatalog(t *testing.T) {
	a := newTestAggregator(t, &fakeRepo{}, nil)
	initial := a.RuntimeSnapshot(0)
	if !initial.CatalogChanged || len(initial.Tools) != 0 || initial.CatalogRevision == 0 {
		t.Fatalf("initial snapshot: %+v", initial)
	}
	session := &fakeSession{tools: []*mcp.Tool{
		{Name: "z", InputSchema: objectSchema()},
		{Name: "a", Description: "before", InputSchema: objectSchema()},
	}}
	up := &upstream{cfg: domain.McpServer{ID: 1, Name: "fs"}, generation: 1}
	a.ups[1] = up
	a.finishConnect(1, 1, session, session.tools, nil)
	added := a.RuntimeSnapshot(initial.CatalogRevision)
	if !added.CatalogChanged || len(added.Tools) != 2 || added.Tools[0].PublicName != "fs__a" {
		t.Fatalf("added snapshot: %+v", added)
	}
	added.Tools[0].Description = "caller mutation"
	if a.ToolCatalog()[0].Description != "before" {
		t.Fatal("snapshot must not expose the cached slice")
	}
	for range 60 {
		unchanged := a.RuntimeSnapshot(added.CatalogRevision)
		data, err := json.Marshal(unchanged)
		if err != nil || unchanged.CatalogChanged || unchanged.Tools != nil || bytes.Contains(data, []byte(`"tools"`)) {
			t.Fatalf("unchanged catalog must not be transmitted: %s, %v", data, err)
		}
	}
	// 内容完全相同的重复通知不应让客户端的目录失效。
	a.refreshTools(1, session, up.generation)
	if a.RuntimeSnapshot(added.CatalogRevision).CatalogChanged {
		t.Fatal("unchanged tool refresh advanced revision")
	}
	session.tools[1] = &mcp.Tool{Name: "a", Description: "after", InputSchema: map[string]any{
		"type": "object", "required": []string{"path"},
	}}
	a.refreshTools(1, session, up.generation)
	changed := a.RuntimeSnapshot(added.CatalogRevision)
	if !changed.CatalogChanged || len(changed.Tools) != 2 || changed.Tools[0].Description != "after" || !strings.Contains(changed.Tools[0].InputSchema, "required") {
		t.Fatalf("same-count content change was lost: %+v", changed)
	}
	a.mu.Lock()
	up.err = "status-only change"
	a.mu.Unlock()
	status := a.RuntimeSnapshot(changed.CatalogRevision)
	if status.CatalogChanged || status.Statuses[0].Error != "status-only change" {
		t.Fatalf("status changes should not resend tools: %+v", status)
	}
	a.mu.Lock()
	a.detachLocked(up)
	a.flushCatalogLocked()
	up.state = StateDisabled
	a.mu.Unlock()
	removed := a.RuntimeSnapshot(changed.CatalogRevision)
	if !removed.CatalogChanged || len(removed.Tools) != 0 || len(a.ToolCatalog()) != 0 {
		t.Fatalf("empty catalog must invalidate clients: %+v", removed)
	}
	a.mu.Lock()
	a.teardownLocked(1)
	a.flushCatalogLocked()
	a.mu.Unlock()
	deleted := a.RuntimeSnapshot(removed.CatalogRevision)
	if deleted.CatalogChanged || len(deleted.Statuses) != 0 {
		t.Fatalf("deleting an empty upstream changes only statuses: %+v", deleted)
	}
}

// 一次同步里批量停用多个上游时，目录只重建一次，客户端也只需重拉一次工具清单。
func TestSyncRebuildsCatalogOnceWhenDetachingMultipleUpstreams(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{
		{ID: 1, Name: "a", Type: domain.ServerTypeStdio, Command: "x", Enabled: true},
		{ID: 2, Name: "b", Type: domain.ServerTypeStdio, Command: "x", Enabled: true},
	}}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		return &fakeSession{tools: []*mcp.Tool{{Name: "t", InputSchema: objectSchema()}}}, nil
	})
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)
	waitForState(t, agg, 2, StateConnected)
	before := agg.RuntimeSnapshot(0).CatalogRevision

	_ = repo.SetEnabled(context.Background(), 1, false)
	_ = repo.SetEnabled(context.Background(), 2, false)
	_ = agg.Sync(context.Background())
	if after := agg.RuntimeSnapshot(0); after.CatalogRevision != before+1 || len(after.Tools) != 0 {
		t.Fatalf("expected one catalog change, revision %d -> %d, tools=%d", before, after.CatalogRevision, len(after.Tools))
	}
}

// finishConnect 释放锁后不能再读 up.cfg：同一时刻 syncOnce 可能正在持锁改写它。
func TestFinishConnectDoesNotReadConfigOutsideLock(t *testing.T) {
	for range 50 {
		repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "a", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
		agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
			return &fakeSession{}, nil
		})
		_ = agg.Sync(context.Background())
		waitForState(t, agg, 1, StateConnected)
		_ = repo.SetEnabled(context.Background(), 1, false)
		_ = agg.Sync(context.Background())
	}
}
