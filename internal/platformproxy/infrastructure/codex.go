package infrastructure

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
	platformproxydomain "portico/internal/platformproxy/domain"
)

const (
	codexSettingsDir       = ".codex"
	codexConfigFileName    = "config.toml"
	codexAuthFileName      = "auth.json"
	codexProviderKey       = "portico"
	legacyCodexProviderKey = "code-switch"
	codexEnvKey            = "OPENAI_API_KEY"
	codexWireAPI           = "responses"
	codexTokenValue        = "portico"
)

var codexConfigFieldSpecs = []tomlFieldSpec{
	{Key: "model_provider", Path: []string{"model_provider"}},
	{Key: "model_providers.portico.name", Path: []string{"model_providers", "portico", "name"}},
	{Key: "model_providers.portico.base_url", Path: []string{"model_providers", "portico", "base_url"}},
	{Key: "model_providers.portico.wire_api", Path: []string{"model_providers", "portico", "wire_api"}},
	{Key: "model_providers.portico.requires_openai_auth", Path: []string{"model_providers", "portico", "requires_openai_auth"}},
	// Keep legacy fields in snapshots so Enable removes the old provider and
	// Disable can still restore configurations created by Code Switch.
	{Key: "model_providers.code-switch.name", Path: []string{"model_providers", "code-switch", "name"}},
	{Key: "model_providers.code-switch.base_url", Path: []string{"model_providers", "code-switch", "base_url"}},
	{Key: "model_providers.code-switch.wire_api", Path: []string{"model_providers", "code-switch", "wire_api"}},
	{Key: "model_providers.code-switch.requires_openai_auth", Path: []string{"model_providers", "code-switch", "requires_openai_auth"}},
}

var codexAuthFieldPaths = []string{codexEnvKey}

type CodexManager struct {
	relayAddr  string
	stateStore proxyStateStore
	mu         sync.RWMutex
}

func NewCodexManager(relayAddr string) *CodexManager {
	return &CodexManager{relayAddr: relayAddr, stateStore: newProxyStateStore("codex")}
}

func (m *CodexManager) ProxyStatus() (platformproxydomain.Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := platformproxydomain.Status{Enabled: false, BaseURL: m.baseURL()}
	state, exists, err := m.stateStore.load()
	if err != nil {
		return status, err
	}
	if exists && !state.Active {
		return status, nil
	}
	config, err := m.readConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status, nil
		}
		return status, err
	}
	providerKey := codexProviderKey
	provider, ok := config.ModelProviders[providerKey]
	if !ok {
		providerKey = legacyCodexProviderKey
		provider, ok = config.ModelProviders[providerKey]
	}
	if !ok {
		return status, nil
	}
	if strings.EqualFold(config.ModelProvider, providerKey) && strings.EqualFold(provider.BaseURL, m.baseURL()) {
		status.Enabled = true
	}
	return status, nil
}

func (m *CodexManager) EnableProxy() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	configPath, err := m.configPath()
	if err != nil {
		return err
	}
	authPath, err := m.authPath()
	if err != nil {
		return err
	}
	configData, configExisted, err := readConfigFile(configPath, nil)
	if err != nil {
		return err
	}
	authData, authExisted, err := readConfigFile(authPath, []byte("{}\n"))
	if err != nil {
		return err
	}
	state, stateExists, err := m.stateStore.load()
	if err != nil {
		return err
	}
	newCycle := !stateExists || !state.Active
	if newCycle {
		configSnapshot, err := captureCodexConfigSnapshot(configData, configExisted)
		if err != nil {
			return err
		}
		authFields, err := readJSONFieldValues(authData, codexAuthFieldPaths)
		if err != nil {
			return err
		}
		state = proxyState{Active: true, Files: map[string]proxyFileSnapshot{
			"config": configSnapshot,
			"auth":   {Existed: authExisted, Fields: authFields},
		}}
	} else if _, _, err := m.codexSnapshots(state); err != nil {
		return err
	}

	// 两个候选文件都成功计算并校验后才保存快照或开始写盘。
	configUpdated, err := m.applyCodexDesiredConfig(configData)
	if err != nil {
		return err
	}
	authUpdated, err := applyJSONFieldEdits(authData, []jsonFieldEdit{{
		Path: codexEnvKey, Raw: strconv.Quote(codexTokenValue), Present: true,
	}})
	if err != nil {
		return err
	}
	if newCycle {
		if err := m.stateStore.save(state); err != nil {
			return err
		}
	}
	configChanged := !bytes.Equal(configData, configUpdated)
	if err := writeConfigFileIfChanged(configPath, configData, configUpdated); err != nil {
		return err
	}
	if err := writeConfigFileIfChanged(authPath, authData, authUpdated); err != nil {
		if configChanged {
			if rollbackErr := m.rollbackCodexConfig(configPath, state); rollbackErr != nil {
				return fmt.Errorf("写入 Codex auth.json 失败: %w；回滚 config.toml 也失败: %v", err, rollbackErr)
			}
		}
		return fmt.Errorf("写入 Codex auth.json 失败: %w", err)
	}
	return nil
}

func (m *CodexManager) DisableProxy() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists, err := m.stateStore.load()
	if err != nil {
		return err
	}
	if exists && !state.Active {
		return nil
	}
	if !exists {
		return m.stateStore.save(proxyState{Active: false})
	}
	configSnapshot, authSnapshot, err := m.codexSnapshots(state)
	if err != nil {
		return err
	}
	configPath, err := m.configPath()
	if err != nil {
		return err
	}
	authPath, err := m.authPath()
	if err != nil {
		return err
	}
	configData, configExisted, err := readConfigFile(configPath, nil)
	if err != nil {
		return err
	}
	authData, authExisted, err := readConfigFile(authPath, []byte("{}\n"))
	if err != nil {
		return err
	}
	configUpdate, err := m.codexConfigRestoreUpdate(configPath, configData, configExisted, configSnapshot)
	if err != nil {
		return err
	}
	authUpdate, err := codexAuthRestoreUpdate(authPath, authData, authExisted, authSnapshot)
	if err != nil {
		return err
	}

	// 先恢复 auth，再恢复决定 ProxyStatus 的 config。任一失败都保留 active
	// 快照供重试；config 失败时把 auth 补偿回本次关闭前的内容。
	if err := authUpdate.apply(); err != nil {
		return err
	}
	if err := configUpdate.apply(); err != nil {
		if authUpdate.changesFile() {
			if rollbackErr := writeConfigFileIfChanged(authPath, nil, authData); rollbackErr != nil {
				return fmt.Errorf("恢复 Codex config.toml 失败: %w；回滚 auth.json 也失败: %v", err, rollbackErr)
			}
		}
		return err
	}
	return m.stateStore.save(proxyState{Active: false})
}

func (m *CodexManager) applyCodexDesiredConfig(data []byte) ([]byte, error) {
	values := map[string]tomlFieldValue{
		"model_provider":                               {Present: true, Raw: strconv.Quote(codexProviderKey)},
		"model_providers.portico.name":                 {Present: true, Raw: strconv.Quote(codexProviderKey)},
		"model_providers.portico.base_url":             {Present: true, Raw: strconv.Quote(m.baseURL())},
		"model_providers.portico.wire_api":             {Present: true, Raw: strconv.Quote(codexWireAPI)},
		"model_providers.portico.requires_openai_auth": {Present: true, Raw: "false"},
	}
	return applyCodexConfigValues(data, values)
}

func applyCodexConfigValues(data []byte, values map[string]tomlFieldValue) ([]byte, error) {
	updated := data
	var err error
	for _, spec := range codexConfigFieldSpecs {
		value := values[spec.Key]
		updated, err = applyTOMLFieldEdit(updated, spec.Path, value.Raw, value.Present)
		if err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func captureCodexConfigSnapshot(data []byte, existed bool) (proxyFileSnapshot, error) {
	values, err := readTOMLFieldValues(data, codexConfigFieldSpecs)
	if err != nil {
		return proxyFileSnapshot{}, err
	}
	fields := make(map[string][]string, len(values))
	for key, value := range values {
		if value.Present {
			fields[key] = []string{value.Raw}
		} else {
			fields[key] = nil
		}
	}
	containers, err := codexContainerPresence(data)
	if err != nil {
		return proxyFileSnapshot{}, err
	}
	return proxyFileSnapshot{Existed: existed, Fields: fields, Containers: containers}, nil
}

func codexContainerPresence(data []byte) (map[string]bool, error) {
	root := make(map[string]any)
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	presence := map[string]bool{
		"model_providers":                           false,
		"model_providers." + codexProviderKey:       false,
		"model_providers." + legacyCodexProviderKey: false,
	}
	providersValue, ok := root["model_providers"]
	if !ok {
		return presence, nil
	}
	providers, ok := providersValue.(map[string]any)
	if !ok {
		return nil, errors.New("Codex model_providers 不是表，拒绝覆盖")
	}
	presence["model_providers"] = true
	for _, key := range []string{codexProviderKey, legacyCodexProviderKey} {
		providerValue, ok := providers[key]
		if !ok {
			continue
		}
		if _, ok := providerValue.(map[string]any); !ok {
			return nil, fmt.Errorf("Codex model_providers.%s 不是表，拒绝覆盖", key)
		}
		presence["model_providers."+key] = true
	}
	return presence, nil
}

func (m *CodexManager) codexSnapshots(state proxyState) (proxyFileSnapshot, proxyFileSnapshot, error) {
	config, ok := state.Files["config"]
	if !ok {
		return proxyFileSnapshot{}, proxyFileSnapshot{}, errors.New("Codex 接入快照缺少 config.toml 原值")
	}
	auth, ok := state.Files["auth"]
	if !ok {
		return proxyFileSnapshot{}, proxyFileSnapshot{}, errors.New("Codex 接入快照缺少 auth.json 原值")
	}
	if _, err := codexConfigRestoreValues(config); err != nil {
		return proxyFileSnapshot{}, proxyFileSnapshot{}, err
	}
	if _, err := jsonSnapshotEdits(auth.Fields, codexAuthFieldPaths); err != nil {
		return proxyFileSnapshot{}, proxyFileSnapshot{}, err
	}
	return config, auth, nil
}

func codexConfigRestoreValues(snapshot proxyFileSnapshot) (map[string]tomlFieldValue, error) {
	values := make(map[string]tomlFieldValue, len(codexConfigFieldSpecs))
	for _, spec := range codexConfigFieldSpecs {
		rawValues := snapshot.Fields[spec.Key]
		if len(rawValues) > 1 {
			return nil, fmt.Errorf("Codex 字段 %s 的快照含有多个值", spec.Key)
		}
		value := tomlFieldValue{Present: len(rawValues) == 1}
		if value.Present {
			value.Raw = rawValues[0]
			if _, err := parseTOMLEditLiteral(value.Raw); err != nil {
				return nil, fmt.Errorf("Codex 字段 %s 的快照 literal 无效: %w", spec.Key, err)
			}
		}
		values[spec.Key] = value
	}
	return values, nil
}

type codexFileUpdate struct {
	path          string
	before        []byte
	after         []byte
	currentExists bool
	remove        bool
}

func (u codexFileUpdate) changesFile() bool {
	if u.remove {
		return u.currentExists
	}
	return !bytes.Equal(u.before, u.after)
}

func (u codexFileUpdate) apply() error {
	if u.remove {
		if !u.currentExists {
			return nil
		}
		return removeConfigFile(u.path)
	}
	return writeConfigFileIfChanged(u.path, u.before, u.after)
}

func (m *CodexManager) codexConfigRestoreUpdate(path string, data []byte, currentExists bool, snapshot proxyFileSnapshot) (codexFileUpdate, error) {
	values, err := codexConfigRestoreValues(snapshot)
	if err != nil {
		return codexFileUpdate{}, err
	}
	updated, err := applyCodexConfigValues(data, values)
	if err != nil {
		return codexFileUpdate{}, err
	}
	var prunePaths [][]string
	if !snapshot.Containers["model_providers.portico"] {
		prunePaths = append(prunePaths, []string{"model_providers", "portico"})
	}
	if !snapshot.Containers["model_providers.code-switch"] {
		prunePaths = append(prunePaths, []string{"model_providers", "code-switch"})
	}
	if !snapshot.Containers["model_providers"] {
		prunePaths = append(prunePaths, []string{"model_providers"})
	}
	if len(prunePaths) != 0 {
		updated, err = pruneTOMLContainers(updated, prunePaths)
		if err != nil {
			return codexFileUpdate{}, err
		}
	}
	update := codexFileUpdate{path: path, before: data, after: updated, currentExists: currentExists}
	if !snapshot.Existed && len(bytes.TrimSpace(updated)) == 0 {
		update.remove = true
	}
	return update, nil
}

func codexAuthRestoreUpdate(path string, data []byte, currentExists bool, snapshot proxyFileSnapshot) (codexFileUpdate, error) {
	edits, err := jsonSnapshotEdits(snapshot.Fields, codexAuthFieldPaths)
	if err != nil {
		return codexFileUpdate{}, err
	}
	updated, err := applyJSONFieldEdits(data, edits)
	if err != nil {
		return codexFileUpdate{}, err
	}
	update := codexFileUpdate{path: path, before: data, after: updated, currentExists: currentExists}
	if !snapshot.Existed {
		empty, err := jsonRootIsEmpty(updated)
		if err != nil {
			return codexFileUpdate{}, err
		}
		update.remove = empty
	}
	return update, nil
}

func (m *CodexManager) rollbackCodexConfig(path string, state proxyState) error {
	configSnapshot, _, err := m.codexSnapshots(state)
	if err != nil {
		return err
	}
	data, existed, err := readConfigFile(path, nil)
	if err != nil {
		return err
	}
	update, err := m.codexConfigRestoreUpdate(path, data, existed, configSnapshot)
	if err != nil {
		return err
	}
	return update.apply()
}

func (m *CodexManager) readConfig() (*codexConfig, error) {
	settingsPath, err := m.configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}
	var cfg codexConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.ModelProviders == nil {
		cfg.ModelProviders = make(map[string]codexProvider)
	}
	return &cfg, nil
}

func (m *CodexManager) configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, codexSettingsDir, codexConfigFileName), nil
}

func (m *CodexManager) authPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, codexSettingsDir, codexAuthFileName), nil
}

func (m *CodexManager) baseURL() string {
	return normalizeRelayBaseURL(m.relayAddr)
}

type codexConfig struct {
	ModelProvider  string                   `toml:"model_provider"`
	ModelProviders map[string]codexProvider `toml:"model_providers"`
}

type codexProvider struct {
	BaseURL string `toml:"base_url"`
}

// stripRedundantTableHeaders 剔除 go-toml v2 序列化嵌套 map 时多产生的
// 空父表头（如裸的 [model_providers] / [mcp_servers] 行）。仅供 MCP 写入使用。
func stripRedundantTableHeaders(data []byte, tables ...string) []byte {
	redundant := make(map[string]bool, len(tables))
	for _, table := range tables {
		redundant["["+table+"]"] = true
	}
	lines := strings.Split(string(data), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if redundant[strings.TrimSpace(line)] {
			continue
		}
		result = append(result, line)
	}
	return []byte(strings.Join(result, "\n"))
}
