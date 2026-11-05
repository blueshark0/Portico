package application

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- 假存储 ----

type fakeRepo struct {
	mu        sync.Mutex
	servers   []domain.McpServer
	nextID    int64
	listCalls atomic.Int32
	listErr   error
}

func (r *fakeRepo) List(context.Context) ([]domain.McpServer, error) {
	r.listCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]domain.McpServer, len(r.servers))
	copy(out, r.servers)
	return out, nil
}

func (r *fakeRepo) Create(_ context.Context, srv *domain.McpServer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	srv.ID = r.nextID
	r.servers = append(r.servers, *srv)
	return nil
}

func (r *fakeRepo) Update(_ context.Context, srv domain.McpServer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.servers {
		if s.ID == srv.ID {
			r.servers[i] = srv
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.servers {
		if s.ID == id {
			r.servers = append(r.servers[:i], r.servers[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeRepo) SetEnabled(_ context.Context, id int64, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.servers {
		if s.ID == id {
			r.servers[i].Enabled = enabled
			return nil
		}
	}
	return errors.New("not found")
}

// ---- 假上游会话 ----

type fakeSession struct {
	tools        []*mcp.Tool
	callFn       func(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	closeCalls   *atomic.Int32
	listToolsErr error
}

func (f *fakeSession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	if f.listToolsErr != nil {
		return nil, f.listToolsErr
	}
	return &mcp.ListToolsResult{Tools: f.tools}, nil
}

func (f *fakeSession) Tools(context.Context, *mcp.ListToolsParams) iter.Seq2[*mcp.Tool, error] {
	return func(yield func(*mcp.Tool, error) bool) {
		if f.listToolsErr != nil {
			yield(nil, f.listToolsErr)
			return
		}
		for _, t := range f.tools {
			if !yield(t, nil) {
				return
			}
		}
	}
}

func (f *fakeSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return f.callFn(ctx, params)
}

func (f *fakeSession) Close() error {
	if f.closeCalls != nil {
		f.closeCalls.Add(1)
	}
	return nil
}

func objectSchema() map[string]any { return map[string]any{"type": "object"} }

func newTestAggregator(t *testing.T, repo *fakeRepo, connect Connector) *Aggregator {
	t.Helper()
	agg := NewAggregator(repo, AggregatorOptions{Connector: connect})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = agg.Close(ctx)
	})
	return agg
}

func echoCall(params *mcp.CallToolParams) *mcp.CallToolResult {
	raw, _ := json.Marshal(params.Arguments)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}

// 全链路：假上游 → 聚合器 → streamable HTTP → 真实 MCP 客户端。
func TestAggregatorEndToEnd(t *testing.T) {
	repo := &fakeRepo{}
	srv := domain.McpServer{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true, TimeoutSec: 5}
	repo.servers = append(repo.servers, srv)

	fake := &fakeSession{
		tools: []*mcp.Tool{
			{Name: "read file", Description: "读文件", InputSchema: objectSchema()},
			{Name: "write", Description: "写文件", InputSchema: objectSchema()},
		},
		callFn: func(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			return echoCall(params), nil
		},
	}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		return fake, nil
	})

	if err := agg.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// connect 是异步的，等待 connected
	waitForState(t, agg, 1, StateConnected)

	ts := httptest.NewServer(agg.Handler())
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("工具数: %d", len(tools.Tools))
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if !names["fs__read_file"] || !names["fs__write"] {
		t.Fatalf("命名空间化失败: %v", names)
	}

	// 调用透传：参数原样到达假上游
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "fs__read_file", Arguments: json.RawMessage(`{"path":"/tmp/x"}`),
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	got := res.Content[0].(*mcp.TextContent).Text
	if got != `{"path":"/tmp/x"}` {
		t.Fatalf("参数透传失真: %s", got)
	}

	// 禁用 → 工具摘除 + 状态 disabled
	if err := repo.SetEnabled(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := agg.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	waitForState(t, agg, 1, StateDisabled)
	tools, err = session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 0 {
		t.Fatalf("禁用后工具应清空: %d", len(tools.Tools))
	}

	// 删除 → 状态消失
	if err := repo.Delete(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := agg.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	for _, st := range agg.Statuses() {
		if st.ServerID == 1 {
			t.Fatalf("删除后仍有状态: %+v", st)
		}
	}
}

// 工具级 IsError 原样透传，不判离线。
func TestToolLevelErrorPassthrough(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	fake := &fakeSession{
		tools: []*mcp.Tool{{Name: "boom", InputSchema: objectSchema()}},
		callFn: func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "bad input"}}}, nil
		},
	}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) { return fake, nil })
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)

	res := callThrough(t, agg, "fs__boom", `{"x":1}`)
	if !res.IsError {
		t.Fatal("IsError 应透传")
	}
	if st := statusOf(agg, 1); st.State != StateConnected {
		t.Fatalf("工具级错误不应判离线: %+v", st)
	}
}

// 传输层错误 → 判 failed + 摘除工具 + 退避重连。
func TestTransportErrorTriggersReconnect(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	var mu sync.Mutex
	broken := false
	fake := &fakeSession{
		tools: []*mcp.Tool{{Name: "t", InputSchema: objectSchema()}},
		callFn: func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			if broken {
				return nil, errors.New("connection reset")
			}
			return echoCall(&mcp.CallToolParams{}), nil
		},
	}
	closeCount := &atomic.Int32{}
	fake.closeCalls = closeCount
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) { return fake, nil })
	agg.now = time.Now
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)

	mu.Lock()
	broken = true
	mu.Unlock()
	res := callThrough(t, agg, "fs__t", `{}`)
	if !res.IsError {
		t.Fatal("传输错误应返回 IsError 结果")
	}
	if st := statusOf(agg, 1); st.State != StateFailed {
		t.Fatalf("应判 failed: %+v", st)
	}
	// 旧会话异步关闭，轮询等待
	deadline := time.Now().Add(2 * time.Second)
	for closeCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if closeCount.Load() == 0 {
		t.Fatal("failed 后应关闭旧会话")
	}

	// 修复后到期重连
	mu.Lock()
	broken = false
	mu.Unlock()
	agg.mu.Lock()
	agg.ups[1].nextRetry = time.Now().Add(-time.Second)
	agg.mu.Unlock()
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)
}

// 连接失败 → failed 态 + 错误信息。
func TestConnectFailure(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "bad", Type: domain.ServerTypeHTTP, URL: "http://x", Enabled: true}}}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		return nil, errors.New("dial refused")
	})
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateFailed)
	if statusOf(agg, 1).Error == "" {
		t.Fatal("failed 态应带错误信息")
	}
}

// 配置指纹变化 → 旧工具摘除、新工具注册。
func TestConfigChangeReconnect(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	version := 1
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		toolName := "v1tool"
		if version == 2 {
			toolName = "v2tool"
		}
		return &fakeSession{
			tools: []*mcp.Tool{{Name: toolName, InputSchema: objectSchema()}},
			callFn: func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
				return echoCall(&mcp.CallToolParams{}), nil
			},
		}, nil
	})
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)
	if !hasTool(agg, "fs__v1tool") {
		t.Fatal("v1 工具应已注册")
	}

	version = 2
	_ = repo.Update(context.Background(), domain.McpServer{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "y", Enabled: true})
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)
	if hasTool(agg, "fs__v1tool") || !hasTool(agg, "fs__v2tool") {
		t.Fatalf("配置变更后工具集应更新: %v", agg.tools)
	}
}

// 并发调用与重配置交错，依赖 -race 检测。
func TestConcurrentCallAndSync(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	fake := &fakeSession{
		tools: []*mcp.Tool{{Name: "t", InputSchema: objectSchema()}},
		callFn: func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			return echoCall(&mcp.CallToolParams{}), nil
		},
	}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) { return fake, nil })
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				callThrough(t, agg, "fs__t", `{}`)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = agg.Sync(context.Background())
		}()
	}
	wg.Wait()
}

func TestTestConnection(t *testing.T) {
	repo := &fakeRepo{}
	agg := newTestAggregator(t, repo, func(_ context.Context, cfg domain.McpServer) (UpstreamSession, error) {
		if cfg.Name == "bad" {
			return nil, errors.New("no")
		}
		return &fakeSession{
			tools:  []*mcp.Tool{{Name: "a", InputSchema: objectSchema()}, {Name: "b", InputSchema: objectSchema()}},
			callFn: func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) { return nil, nil },
		}, nil
	})
	res, err := agg.TestConnection(context.Background(), domain.McpServer{Name: "ok", Type: domain.ServerTypeHTTP, URL: "http://x"})
	if err != nil || !res.Success || res.ToolCount != 2 {
		t.Fatalf("TestConnection 成功路径: %+v err=%v", res, err)
	}
	res, _ = agg.TestConnection(context.Background(), domain.McpServer{Name: "bad", Type: domain.ServerTypeHTTP, URL: "http://x"})
	if res.Success || res.Error == "" {
		t.Fatalf("TestConnection 失败路径: %+v", res)
	}
}

// 真实 stdio 链路：helper 进程跑一个真 MCP server，验证 CommandTransport 与进程回收。
func TestStdioConnectorHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		if os.Getenv("PYTHONHOME") != "" || os.Getenv("PYTHONPATH") != "" {
			os.Exit(2)
		}
		server := mcp.NewServer(&mcp.Implementation{Name: "helper", Version: "v0"}, nil)
		server.AddTool(&mcp.Tool{Name: "ping", InputSchema: objectSchema()},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil
			})
		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	appDir := t.TempDir()
	t.Setenv("APPDIR", appDir)
	t.Setenv("PYTHONHOME", appDir+"/usr")
	t.Setenv("PYTHONPATH", appDir+"/usr/share/pyshared")
	connect := defaultConnector("test", nil)
	cfg := domain.McpServer{
		Name: "helper", Type: domain.ServerTypeStdio,
		Command: os.Args[0], Args: []string{"-test.run=TestStdioConnectorHelperProcess"},
		Env: map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := connect(ctx, cfg)
	if err != nil {
		t.Fatalf("stdio connect: %v", err)
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil {
		t.Fatalf("stdio call: %v", err)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; got != "pong" {
		t.Fatalf("got %q", got)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Close 返回即完成 stdin→SIGTERM→SIGKILL 序列，无需额外断言进程退出
}

// ---- 工具函数 ----

func waitForState(t *testing.T, agg *Aggregator, id int64, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if statusOf(agg, id).State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待状态 %s 超时，当前: %+v", want, statusOf(agg, id))
}

func statusOf(agg *Aggregator, id int64) UpstreamStatus {
	for _, st := range agg.Statuses() {
		if st.ServerID == id {
			return st
		}
	}
	return UpstreamStatus{}
}

func hasTool(agg *Aggregator, name string) bool {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	_, ok := agg.tools[name]
	return ok
}

// callThrough 直接走聚合器内部的转发 handler（不经 HTTP 层）。
func callThrough(t *testing.T, agg *Aggregator, publicName, args string) *mcp.CallToolResult {
	t.Helper()
	agg.mu.Lock()
	ref, ok := agg.tools[publicName]
	agg.mu.Unlock()
	if !ok {
		t.Fatalf("工具未注册: %s", publicName)
	}
	handler := agg.makeCallHandler(ref.serverID, ref.upstreamName)
	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{Name: publicName, Arguments: json.RawMessage(args)}
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler 协议级错误: %v", err)
	}
	return res
}

// 跨上游公共名冲突：后者不得覆盖前者的注册（防御无校验进入存储的历史数据；
// 新数据由 domain.Validate 的禁 "__" 规则在入口拦截）。
func TestCrossServerCollisionSkipped(t *testing.T) {
	repo := &fakeRepo{}
	agg := newTestAggregator(t, repo, nil)
	up1 := &upstream{cfg: domain.McpServer{ID: 1, Name: "a"}, state: StateConnected}
	up2 := &upstream{cfg: domain.McpServer{ID: 2, Name: "a__b"}, state: StateConnected}
	agg.ups[1], agg.ups[2] = up1, up2

	agg.mu.Lock()
	agg.registerToolsLocked(up1, []*mcp.Tool{{Name: "b__c", InputSchema: objectSchema()}})
	agg.registerToolsLocked(up2, []*mcp.Tool{{Name: "c", InputSchema: objectSchema()}})
	agg.mu.Unlock()

	agg.mu.Lock()
	ref, ok := agg.tools["a__b__c"]
	agg.mu.Unlock()
	if !ok || ref.serverID != 1 {
		t.Fatalf("冲突名应保留先注册者: %+v ok=%v", ref, ok)
	}
	// 后者的同名工具被跳过，不影响其状态
	if len(up2.toolNames) != 0 {
		t.Fatalf("冲突工具不应计入后者: %v", up2.toolNames)
	}
}

// 调用方取消/本工具超时不应判离线：健康上游的工具与会话必须保留。
func TestCallerCancelDoesNotMarkFailed(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true, TimeoutSec: 30}}}
	fake := &fakeSession{
		tools: []*mcp.Tool{{Name: "slow", InputSchema: objectSchema()}},
		callFn: func(ctx context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			<-ctx.Done() // 模拟慢上游，直到调用方上下文结束
			return nil, ctx.Err()
		},
	}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) { return fake, nil })
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	handler := agg.makeCallHandler(1, "slow")
	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{Name: "fs__slow", Arguments: json.RawMessage(`{}`)}
	res, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler 协议级错误: %v", err)
	}
	if !res.IsError {
		t.Fatal("取消应返回 IsError 结果")
	}
	if st := statusOf(agg, 1); st.State != StateConnected {
		t.Fatalf("取消不应判离线: %+v", st)
	}
	if !hasTool(agg, "fs__slow") {
		t.Fatal("取消不应摘除工具")
	}
}

// Close 之后 Sync 不得再产生新连接（竞态窗口已由 closed 标志封死）。
func TestSyncAfterCloseIsNoop(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	var connectCalls atomic.Int32
	agg := NewAggregator(repo, AggregatorOptions{Connector: func(context.Context, domain.McpServer) (UpstreamSession, error) {
		connectCalls.Add(1)
		return &fakeSession{}, nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := agg.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := agg.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if connectCalls.Load() != 0 {
		t.Fatalf("Close 后不应再发起连接，实际 %d 次", connectCalls.Load())
	}
}

// 初始化成功但 tools/list 失败：测试连接必须如实报失败，不能计成工具数。
func TestTestConnectionReportsListError(t *testing.T) {
	repo := &fakeRepo{}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) {
		return &fakeSession{listToolsErr: errors.New("list boom")}, nil
	})
	res, err := agg.TestConnection(context.Background(), domain.McpServer{Name: "x", Type: domain.ServerTypeHTTP, URL: "http://x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Fatal("tools/list 失败时 TestConnection 不得报成功")
	}
	if res.Error == "" {
		t.Fatal("应携带错误信息")
	}
}

// 上游 tools/list_changed → 不中断连接地刷新工具注册表（新增与删除都生效）。
func TestRefreshToolsOnListChanged(t *testing.T) {
	repo := &fakeRepo{servers: []domain.McpServer{{ID: 1, Name: "fs", Type: domain.ServerTypeStdio, Command: "x", Enabled: true}}}
	fake := &fakeSession{
		tools: []*mcp.Tool{{Name: "a", InputSchema: objectSchema()}},
		callFn: func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			return echoCall(&mcp.CallToolParams{}), nil
		},
	}
	agg := newTestAggregator(t, repo, func(context.Context, domain.McpServer) (UpstreamSession, error) { return fake, nil })
	_ = agg.Sync(context.Background())
	waitForState(t, agg, 1, StateConnected)
	if !hasTool(agg, "fs__a") {
		t.Fatal("初始工具应已注册")
	}

	// 上游新增 b、移除 a
	fake.tools = []*mcp.Tool{{Name: "b", InputSchema: objectSchema()}}
	agg.onUpstreamToolsChanged(1)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hasTool(agg, "fs__b") && !hasTool(agg, "fs__a") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("list_changed 后注册表未刷新: a=%v b=%v", hasTool(agg, "fs__a"), hasTool(agg, "fs__b"))
}
