// Package application MCP 聚合网关的应用层：上游连接池、工具注册表与转发。
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"portico/internal/mcpgateway/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Repository 消费端定义的存储接口，infrastructure.SQLiteStore 结构性满足。
type Repository interface {
	List(ctx context.Context) ([]domain.McpServer, error)
	Create(ctx context.Context, server *domain.McpServer) error
	Update(ctx context.Context, server domain.McpServer) error
	Delete(ctx context.Context, id int64) error
	SetEnabled(ctx context.Context, id int64, enabled bool) error
}

// UpstreamSession 窄化 *mcp.ClientSession 便于测试注入假上游。
type UpstreamSession interface {
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	// Tools 自动翻页遍历全部工具。
	Tools(ctx context.Context, params *mcp.ListToolsParams) iter.Seq2[*mcp.Tool, error]
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Close() error
}

// Connector 建立到上游的会话；测试可注入假实现。
type Connector func(ctx context.Context, cfg domain.McpServer) (UpstreamSession, error)

// 上游连接状态机。
const (
	StateConnecting = "connecting"
	StateConnected  = "connected"
	StateFailed     = "failed"
	StateDisabled   = "disabled"
)

// UpstreamStatus 供 UI 轮询的状态快照。
type UpstreamStatus struct {
	ServerID  int64  `json:"serverId"`
	State     string `json:"state"`
	ToolCount int    `json:"toolCount"`
	Error     string `json:"error,omitempty"`
}

// ToolInfo 单个已发布工具的快照，供 UI 工具清单与诊断页展示。
// InputSchema 是发布到下游的（已消毒）JSON Schema 字符串，与 tools/list 下发一致。
type ToolInfo struct {
	ServerID     int64  `json:"serverId"`
	ServerName   string `json:"serverName"`
	PublicName   string `json:"publicName"`
	UpstreamName string `json:"upstreamName"`
	Description  string `json:"description"`
	InputSchema  string `json:"inputSchema"`
	ReadOnly     bool   `json:"readOnly"`
	Destructive  bool   `json:"destructive"`
}

const (
	connectTimeout = 10 * time.Second
	// sessionTimeout 与 Codex 从不 DELETE 会话（实测 1.8% 关闭率）的行为互补，
	// 同时缓解 go-sdk #499 的 abandoned session goroutine 泄漏。
	sessionTimeout = 30 * time.Minute
	// 只有同步失败或存在失败上游时才需要定时唤醒；正常空闲不读数据库。
	reconcileErrorInitialDelay = time.Second
	reconcileErrorMaxDelay     = 30 * time.Second
	maxCallToolTools           = 2048 // Codex MAX_MCP_CATALOG_ITEMS，超过则该端点 tools/list 整体失败
)

// 重连退避：2s→5s→15s→30s 封顶，持续重试（上游可能只是慢启动）。
var backoffSteps = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second}

// 收窄协议版本：2026-07-28 仅接受 stateless 传输，而 stateful 是
// tools/list_changed 推送的前提；收窄后协商落在 2025-11-25，客户端无感。
var supportedProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

type upstream struct {
	cfg         domain.McpServer
	fingerprint string
	session     UpstreamSession
	state       string
	err         string
	toolNames   []string
	toolDetails []ToolInfo // 与 toolNames 同生共灭：注册时填充、摘除时清空
	backoffIdx  int
	nextRetry   time.Time
	generation  int64 // 全局代际号，防止迟到的连接结果落在已变更的记录上
}

// Aggregator MCP 聚合器：单例 mcp.Server + 上游连接池 + 工具注册表。
type Aggregator struct {
	repo      Repository
	connect   Connector
	mcpServer *mcp.Server
	handler   http.Handler

	mu     sync.Mutex
	ups    map[int64]*upstream
	tools  map[string]toolRef
	genSeq int64
	closed bool          // Close 后置位，阻止一切新连接/注册
	syncCh chan struct{} // cap 1，合并连续变更
	stopCh chan struct{}
	wg     sync.WaitGroup

	now func() time.Time // 测试钩子
}

type toolRef struct {
	serverID     int64
	upstreamName string
}

// AggregatorOptions 聚合器选项。
type AggregatorOptions struct {
	AppVersion string
	Connector  Connector // nil → 默认 stdio/http connector
}

// NewAggregator 创建聚合器；opts 零值可用（默认 connector、无版本号）。
func NewAggregator(repo Repository, opts AggregatorOptions) *Aggregator {
	a := &Aggregator{
		repo:   repo,
		ups:    make(map[int64]*upstream),
		tools:  make(map[string]toolRef),
		syncCh: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
		now:    time.Now,
	}
	connect := opts.Connector
	if connect == nil {
		connect = defaultConnector(opts.AppVersion, a.onUpstreamToolsChanged)
	}
	a.connect = connect
	a.mcpServer = mcp.NewServer(
		&mcp.Implementation{Name: "portico", Version: opts.AppVersion},
		&mcp.ServerOptions{
			HasTools:                  true, // 工具在客户端 Connect 之后动态注册，capability 不能靠推断
			SupportedProtocolVersions: supportedProtocolVersions,
		})
	a.handler = mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return a.mcpServer },
		&mcp.StreamableHTTPOptions{SessionTimeout: sessionTimeout})
	return a
}

// Handler 返回挂在 relay 上的 streamable HTTP 入口。
func (a *Aggregator) Handler() http.Handler { return a.handler }

// Start 启动 reconciler goroutine；连接异步进行，不阻塞应用启动。
func (a *Aggregator) Start(ctx context.Context) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.reconcileLoop(ctx)
	}()
	a.NotifyChanged()
}

// NotifyChanged 配置变更后触发一次同步（非阻塞，连续变更自动合并）。
func (a *Aggregator) NotifyChanged() {
	select {
	case a.syncCh <- struct{}{}:
	default:
	}
}

// Sync 同步执行一次 reconciling，供测试直接调用。
func (a *Aggregator) Sync(ctx context.Context) error {
	return a.syncOnce(ctx)
}

func (a *Aggregator) reconcileLoop(ctx context.Context) {
	var retryTimer *time.Timer
	var retryC <-chan time.Time
	defer func() {
		if retryTimer != nil {
			retryTimer.Stop()
		}
	}()
	var syncErrorDelay time.Duration

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-a.syncCh:
		case <-retryC: // 仅失败重连或同步失败重试时启用
		}

		if err := a.syncOnce(ctx); err != nil {
			log.Printf("mcpgateway: sync: %v", err)
			// 数据库暂时不可用不能热循环，也不能永久睡着；成功后清零退避。
			if syncErrorDelay == 0 {
				syncErrorDelay = reconcileErrorInitialDelay
			} else {
				syncErrorDelay *= 2
				if syncErrorDelay > reconcileErrorMaxDelay {
					syncErrorDelay = reconcileErrorMaxDelay
				}
			}
			retryC = resetReconcileTimer(&retryTimer, syncErrorDelay)
			continue
		}
		syncErrorDelay = 0

		if delay, ok := a.nextRetryDelay(); ok {
			retryC = resetReconcileTimer(&retryTimer, delay)
		} else {
			retryC = stopReconcileTimer(&retryTimer)
		}
	}
}

// nextRetryDelay 返回最早一个失败上游的等待时间；没有失败重试时返回 false。
// 只读内存状态，不访问 repo。
func (a *Aggregator) nextRetryDelay() (time.Duration, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var earliest time.Time
	for _, up := range a.ups {
		if up.state != StateFailed || up.nextRetry.IsZero() {
			continue
		}
		if earliest.IsZero() || up.nextRetry.Before(earliest) {
			earliest = up.nextRetry
		}
	}
	if earliest.IsZero() {
		return 0, false
	}
	delay := earliest.Sub(a.now())
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

// resetReconcileTimer 只由 reconciler goroutine 调用，避免并发 Reset 和 stale tick。
func resetReconcileTimer(timer **time.Timer, delay time.Duration) <-chan time.Time {
	if delay < 0 {
		delay = 0
	}
	if *timer == nil {
		*timer = time.NewTimer(delay)
		return (*timer).C
	}
	if !(*timer).Stop() {
		select {
		case <-(*timer).C:
		default:
		}
	}
	(*timer).Reset(delay)
	return (*timer).C
}

func stopReconcileTimer(timer **time.Timer) <-chan time.Time {
	if *timer != nil && !(*timer).Stop() {
		select {
		case <-(*timer).C:
		default:
		}
	}
	return nil
}

// syncOnce 以 repo 当前配置为基准做 diff：
// 消失 → 删除记录；禁用 → 保留记录置为 disabled 并摘除连接；
// 新增/指纹变化/disabled 重新启用/失败到期 → 发起连接。
func (a *Aggregator) syncOnce(ctx context.Context) error {
	servers, err := a.repo.List(ctx)
	if err != nil {
		return err
	}
	desired := make(map[int64]domain.McpServer, len(servers))
	disabled := make(map[int64]domain.McpServer)
	for _, srv := range servers {
		if srv.Enabled {
			desired[srv.ID] = srv
		} else {
			disabled[srv.ID] = srv
		}
	}

	type connectJob struct {
		cfg domain.McpServer
		gen int64
	}
	var jobs []connectJob

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	for id, up := range a.ups {
		cfg, ok := desired[id]
		if ok {
			if up.fingerprint != cfg.Fingerprint() {
				a.teardownLocked(id) // 配置变更：整段重建
			}
			continue
		}
		if cfg, isDisabled := disabled[id]; isDisabled {
			// 禁用：摘除连接但保留记录，Statuses 才能展示 disabled 态
			if up.state != StateDisabled {
				a.detachLocked(up)
				up.cfg = cfg
				up.fingerprint = cfg.Fingerprint()
				up.state = StateDisabled
				up.err = ""
			}
			continue
		}
		a.teardownLocked(id) // 已删除
	}
	for id, cfg := range desired {
		up, ok := a.ups[id]
		if !ok {
			up = &upstream{cfg: cfg, fingerprint: cfg.Fingerprint(), state: StateConnecting}
			a.ups[id] = up
		} else {
			retryDue := up.state == StateFailed && !a.now().Before(up.nextRetry)
			if up.state != StateDisabled && !retryDue {
				continue
			}
			up.state = StateConnecting
			up.err = ""
		}
		a.genSeq++
		up.generation = a.genSeq
		// wg.Add 收敛进锁内：Close 置 closed 后才会 wg.Wait，此后不再有新 Add，
		// 避免 WaitGroup "Add 与 Wait 并发" 的误用。
		a.wg.Add(1)
		jobs = append(jobs, connectJob{cfg, up.generation})
	}
	a.mu.Unlock()

	for _, job := range jobs {
		go func() {
			defer a.wg.Done()
			a.connectAndRegister(job.cfg, job.gen)
		}()
	}
	return nil
}

// connectAndRegister 连接上游并注册其全部工具；结果经锁内代际校验后落账。
func (a *Aggregator) connectAndRegister(cfg domain.McpServer, gen int64) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	session, err := a.connect(ctx, cfg)
	if err != nil {
		a.finishConnect(cfg.ID, gen, nil, nil, fmt.Errorf("连接失败: %w", err))
		return
	}
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			_ = session.Close()
			a.finishConnect(cfg.ID, gen, nil, nil, fmt.Errorf("获取工具列表失败: %w", err))
			return
		}
		tools = append(tools, tool)
	}
	a.finishConnect(cfg.ID, gen, session, tools, nil)
}

func (a *Aggregator) finishConnect(id int64, gen int64, session UpstreamSession, tools []*mcp.Tool, connErr error) {
	a.mu.Lock()
	up, ok := a.ups[id]
	if !ok || up.generation != gen || a.closed {
		// 连接期间配置已变更/禁用/删除，或网关正在关闭，这批结果作废
		a.mu.Unlock()
		if session != nil {
			_ = session.Close()
		}
		return
	}
	if connErr != nil {
		up.state = StateFailed
		up.err = connErr.Error()
		up.nextRetry = a.now().Add(backoffSteps[min(up.backoffIdx, len(backoffSteps)-1)])
		up.backoffIdx++
		name := up.cfg.Name
		a.mu.Unlock()
		log.Printf("mcpgateway: 上游 %q 连接失败: %v", name, connErr)
		// connect 是异步的；本轮 syncOnce 可能早已结束，必须唤醒 loop
		// 让它根据刚落账的 nextRetry 安装重试 timer。
		a.NotifyChanged()
		return
	}

	up.session = session
	up.state = StateConnected
	up.err = ""
	up.backoffIdx = 0
	a.registerToolsLocked(up, tools)
	total := len(a.tools)
	registered := len(up.toolNames)
	a.mu.Unlock()
	if total > maxCallToolTools {
		log.Printf("mcpgateway: 聚合工具数 %d 超过 %d，Codex 客户端 tools/list 将整体失败", total, maxCallToolTools)
	}
	log.Printf("mcpgateway: 上游 %q 已连接，注册 %d 个工具", up.cfg.Name, registered)
}

// registerToolsLocked 把上游工具注册进单例 server 与注册表。调用方必须持有 a.mu。
// 冲突处理：同一上游内消毒后重名 → 跳过；跨上游公共名冲突（注册表已被别的
// serverID 占用）→ 跳过并 log，先注册者保留——避免后者静默替换前者 handler、
// 以及任一上游被禁用时误删共享公共名对应的工具。
func (a *Aggregator) registerToolsLocked(up *upstream, tools []*mcp.Tool) {
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		public := domain.PublicToolName(up.cfg.Name, tool.Name)
		if seen[public] {
			log.Printf("mcpgateway: 上游 %q 工具 %q 消毒后与既有工具重名，跳过", up.cfg.Name, tool.Name)
			continue
		}
		seen[public] = true
		if ref, taken := a.tools[public]; taken && ref.serverID != up.cfg.ID {
			log.Printf("mcpgateway: 公共工具名 %q 已被上游 #%d 占用，跳过上游 %q 的工具 %q",
				public, ref.serverID, up.cfg.Name, tool.Name)
			continue
		}
		schema := sanitizeInputSchema(up.cfg.Name, tool)
		a.mcpServer.AddTool(&mcp.Tool{
			Name:        public,
			Description: tool.Description,
			InputSchema: schema,
			Annotations: tool.Annotations,
		}, a.makeCallHandler(up.cfg.ID, tool.Name))
		up.toolNames = append(up.toolNames, public)
		up.toolDetails = append(up.toolDetails, newToolInfo(up, tool, public, schema))
		a.tools[public] = toolRef{serverID: up.cfg.ID, upstreamName: tool.Name}
	}
}

// newToolInfo 生成工具的 UI 快照；schema 以发布到下游的消毒结果为准。
// schema 序列化失败时降级为空串而不是让整个注册失败。
func newToolInfo(up *upstream, tool *mcp.Tool, public string, schema any) ToolInfo {
	info := ToolInfo{
		ServerID:     up.cfg.ID,
		ServerName:   up.cfg.Name,
		PublicName:   public,
		UpstreamName: tool.Name,
		Description:  tool.Description,
	}
	if ann := tool.Annotations; ann != nil {
		info.ReadOnly = ann.ReadOnlyHint
		if ann.DestructiveHint != nil {
			info.Destructive = *ann.DestructiveHint
		}
	}
	if data, err := json.Marshal(schema); err == nil {
		info.InputSchema = string(data)
	}
	return info
}

// onUpstreamToolsChanged 上游发出 notifications/tools/list_changed 时触发
// 该上游的工具重拉与注册表刷新（不断开连接）。
func (a *Aggregator) onUpstreamToolsChanged(id int64) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	up := a.ups[id]
	if up == nil || up.state != StateConnected || up.session == nil {
		a.mu.Unlock()
		return
	}
	session := up.session
	gen := up.generation
	a.wg.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		a.refreshTools(id, session, gen)
	}()
}

func (a *Aggregator) refreshTools(id int64, session UpstreamSession, gen int64) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			log.Printf("mcpgateway: 上游 #%d 工具列表刷新失败: %v", id, err)
			return
		}
		tools = append(tools, tool)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	up := a.ups[id]
	if up == nil || up.session != session || up.generation != gen || a.closed {
		return // 期间已重连/禁用/删除/关闭，旧会话的刷新结果作废
	}
	if len(up.toolNames) > 0 {
		a.mcpServer.RemoveTools(up.toolNames...)
		for _, name := range up.toolNames {
			delete(a.tools, name)
		}
		up.toolNames = nil
		up.toolDetails = nil
	}
	a.registerToolsLocked(up, tools)
	log.Printf("mcpgateway: 上游 %q 工具列表已刷新，当前 %d 个工具", up.cfg.Name, len(up.toolNames))
}

// detachLocked 摘除上游全部工具并异步关闭会话，但保留记录；
// 同时递增代际使进行中的连接结果作废。调用方必须持有 a.mu。
func (a *Aggregator) detachLocked(up *upstream) {
	a.genSeq++
	up.generation = a.genSeq
	if len(up.toolNames) > 0 {
		a.mcpServer.RemoveTools(up.toolNames...)
		for _, name := range up.toolNames {
			delete(a.tools, name)
		}
		up.toolNames = nil
		up.toolDetails = nil
	}
	session := up.session
	up.session = nil
	if session != nil {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			_ = session.Close() // stdio：SDK 已实现 stdin 关闭→5s→SIGTERM→SIGKILL
		}()
	}
}

// teardownLocked 摘除连接并删除记录。调用方必须持有 a.mu。
func (a *Aggregator) teardownLocked(id int64) {
	up, ok := a.ups[id]
	if !ok {
		return
	}
	a.detachLocked(up)
	delete(a.ups, id)
}

// makeCallHandler 生成转发闭包；每次调用按 serverID 现查会话，重连后自动生效。
func (a *Aggregator) makeCallHandler(serverID int64, upstreamName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a.mu.Lock()
		up := a.ups[serverID]
		var session UpstreamSession
		var timeoutSec int
		if up != nil {
			session = up.session
			timeoutSec = up.cfg.TimeoutSec
		}
		a.mu.Unlock()

		if session == nil {
			return toolErrorResult("上游 MCP 服务当前不可用（未连接）"), nil
		}
		if timeoutSec <= 0 {
			timeoutSec = domain.DefaultTimeoutSec
		}
		callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()

		// Arguments 为 json.RawMessage，赋给 any 后按原始字节转发，不做二次编解码
		res, err := session.CallTool(callCtx, &mcp.CallToolParams{
			Name:      upstreamName,
			Arguments: req.Params.Arguments,
		})
		if err != nil {
			// 我方上下文结束（调用方取消或本工具超时）不代表上游故障：
			// 误拆共享会话会中断该上游上并发的无关调用并重启 stdio 进程。
			// 只有非上下文类错误才按传输故障判离线重连；工具级错误
			// （res.IsError）原样透传不判离线。
			if callCtx.Err() != nil {
				return toolErrorResult(fmt.Sprintf("调用已取消或超时: %v", err)), nil
			}
			a.markFailed(serverID, session, err)
			return toolErrorResult(fmt.Sprintf("调用上游 MCP 服务失败: %v", err)), nil
		}
		return res, nil
	}
}

// markFailed 调用失败时把上游置为 failed 并摘除工具；仅当会话未被替换时生效。
func (a *Aggregator) markFailed(serverID int64, session UpstreamSession, callErr error) {
	a.mu.Lock()
	up, ok := a.ups[serverID]
	if !ok || up.session != session {
		a.mu.Unlock()
		return
	}
	cfg := up.cfg
	a.detachLocked(up)
	up.session = nil
	up.state = StateFailed
	up.err = callErr.Error()
	up.nextRetry = a.now().Add(backoffSteps[0])
	name := cfg.Name
	a.mu.Unlock()
	log.Printf("mcpgateway: 上游 %q 调用失败，转入重连: %v", name, callErr)
	a.NotifyChanged()
}

// ToolCatalog 全部上游已发布工具的快照（按 serverID、公共名排序，输出稳定），
// 供 UI 工具清单与诊断页使用。
func (a *Aggregator) ToolCatalog() []ToolInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ToolInfo, 0, len(a.tools))
	for _, up := range a.ups {
		out = append(out, up.toolDetails...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ServerID != out[j].ServerID {
			return out[i].ServerID < out[j].ServerID
		}
		return out[i].PublicName < out[j].PublicName
	})
	return out
}

// Statuses 全部上游（含 disabled）的状态快照。
func (a *Aggregator) Statuses() []UpstreamStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	statuses := make([]UpstreamStatus, 0, len(a.ups))
	for id, up := range a.ups {
		statuses = append(statuses, UpstreamStatus{
			ServerID:  id,
			State:     up.state,
			ToolCount: len(up.toolNames),
			Error:     up.err,
		})
	}
	return statuses
}

// TestConnection 用独立临时连接探测配置可达性，不进连接池。
func (a *Aggregator) TestConnection(ctx context.Context, cfg domain.McpServer) (domain.TestResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	session, err := a.connect(ctx, cfg)
	if err != nil {
		return domain.TestResult{Success: false, Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	defer func() { _ = session.Close() }()
	count := 0
	for _, err := range session.Tools(ctx, nil) {
		if err != nil {
			// 初始化成功但 tools/list 失败：如实上报失败，不能计成工具数
			return domain.TestResult{Success: false, Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
		}
		count++
	}
	return domain.TestResult{Success: true, ToolCount: count, DurationMs: time.Since(start).Milliseconds()}, nil
}

// Close 停止 reconciler 并关闭全部上游会话（含 stdio 子进程）。
// closed 置位先于拆除：此后 syncOnce 不再产生新连接、finishConnect 丢弃
// 迟到的结果并立即关会话，wg.Add 全部收敛在持锁区段内，因此 wg.Wait
// 返回时不存在漏关的会话。
func (a *Aggregator) Close(ctx context.Context) error {
	select {
	case <-a.stopCh:
	default:
		close(a.stopCh)
	}
	a.mu.Lock()
	a.closed = true
	for id := range a.ups {
		a.teardownLocked(id)
	}
	a.mu.Unlock()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func toolErrorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// sanitizeInputSchema 防御 AddTool 的 panic 约定：InputSchema 必须为 type=object
// 的 JSON Schema。上游 schema 合法时以 json.RawMessage 原样透传保真，
// 否则降级为宽松 object schema 并记录日志。
func sanitizeInputSchema(serverName string, tool *mcp.Tool) any {
	permissive := map[string]any{"type": "object"}
	if tool.InputSchema == nil {
		return permissive
	}
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		log.Printf("mcpgateway: 上游 %q 工具 %q 的 inputSchema 无法序列化，降级为宽松 schema: %v", serverName, tool.Name, err)
		return permissive
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.Type != "object" {
		log.Printf("mcpgateway: 上游 %q 工具 %q 的 inputSchema 缺少 type=object，降级为宽松 schema", serverName, tool.Name)
		return permissive
	}
	return json.RawMessage(data)
}
