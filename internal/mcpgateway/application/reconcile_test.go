package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func waitForRepoLists(t *testing.T, repo *fakeRepo, want int32) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if repo.listCalls.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待 repo.List 次数达到 %d 超时，当前 %d", want, repo.listCalls.Load())
}

func TestReconcileLoopDoesNotPollIdleRepository(t *testing.T) {
	repo := &fakeRepo{}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		return nil, errors.New("不应尝试连接")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agg.Start(ctx)
	waitForRepoLists(t, repo, 1)

	// 超过旧的 1s ticker 周期；没有失败上游时不应再访问 repo。
	time.Sleep(1300 * time.Millisecond)
	if got := repo.listCalls.Load(); got != 1 {
		t.Fatalf("空闲期间 repo.List 被调用 %d 次，want 1", got)
	}
}

func TestReconcileLoopRetriesLateConnectFailure(t *testing.T) {
	oldBackoff := backoffSteps
	backoffSteps = []time.Duration{50 * time.Millisecond}
	t.Cleanup(func() { backoffSteps = oldBackoff })

	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	var attempts atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseFirst)
		}
	}()

	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		if attempts.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return nil, errors.New("temporary connect failure")
		}
		return &fakeSession{tools: []*mcp.Tool{{Name: "t", InputSchema: objectSchema()}}}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agg.Start(ctx)

	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("首次连接未开始")
	}
	waitForRepoLists(t, repo, 1)
	// 让首轮 syncOnce 在连接仍处 connecting 时完成，确保后续失败是异步落账。
	time.Sleep(100 * time.Millisecond)
	released = true
	close(releaseFirst)

	// 不再人工调用 Sync/NotifyChanged；失败分支必须自己唤醒重试调度。
	waitForState(t, agg, 1, StateConnected)
	if got := attempts.Load(); got < 2 {
		t.Fatalf("自动重连次数 = %d, want >= 2", got)
	}
}

func TestReconcileLoopRetriesAfterRepositoryRecovery(t *testing.T) {
	repo := &fakeRepo{listErr: errors.New("temporary db failure")}
	agg := newTestAggregator(t, repo, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agg.Start(ctx)
	waitForRepoLists(t, repo, 1)

	repo.mu.Lock()
	repo.listErr = nil
	repo.mu.Unlock()
	// 首次同步失败后应按退避再次尝试，而不是永久等待下一次配置变更。
	waitForRepoLists(t, repo, 2)
}
