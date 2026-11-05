package infrastructure

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	platformproxydomain "portico/internal/platformproxy/domain"
)

const (
	claudeSettingsDir      = ".claude"
	claudeSettingsFileName = "settings.json"
	claudeAuthTokenValue   = "portico"
	legacyAuthTokenValue   = "code-switch"
)

var claudeProxyFieldPaths = []string{
	"env.ANTHROPIC_AUTH_TOKEN",
	"env.ANTHROPIC_BASE_URL",
}

type ClaudeManager struct {
	relayAddr  string
	stateStore proxyStateStore
	mu         sync.RWMutex
}

func NewClaudeManager(relayAddr string) *ClaudeManager {
	return &ClaudeManager{relayAddr: relayAddr, stateStore: newProxyStateStore("claude")}
}

func (m *ClaudeManager) ProxyStatus() (platformproxydomain.Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.proxyStatus()
}

func isManagedPlaceholderToken(value, current string) bool {
	return strings.EqualFold(value, current) || strings.EqualFold(value, legacyAuthTokenValue)
}

func (m *ClaudeManager) proxyStatus() (platformproxydomain.Status, error) {
	status := platformproxydomain.Status{Enabled: false, BaseURL: m.baseURL()}
	state, exists, err := m.stateStore.load()
	if err != nil {
		return status, err
	}
	if exists && !state.Active {
		return status, nil
	}
	settingsPath, err := m.settingsPath()
	if err != nil {
		return status, err
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status, nil
		}
		return status, err
	}
	var payload claudeSettingsFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return status, nil
	}
	status.Enabled = isManagedPlaceholderToken(payload.Env["ANTHROPIC_AUTH_TOKEN"], claudeAuthTokenValue) &&
		strings.EqualFold(payload.Env["ANTHROPIC_BASE_URL"], m.baseURL())
	return status, nil
}

func (m *ClaudeManager) EnableProxy() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	settingsPath, err := m.settingsPath()
	if err != nil {
		return err
	}
	data, fileExisted, err := readConfigFile(settingsPath, []byte("{}\n"))
	if err != nil {
		return err
	}
	state, stateExists, err := m.stateStore.load()
	if err != nil {
		return err
	}
	newCycle := !stateExists || !state.Active
	if newCycle {
		fields, err := readJSONFieldValues(data, claudeProxyFieldPaths)
		if err != nil {
			return err
		}
		containers, err := jsonContainerPresence(data, []string{"env"})
		if err != nil {
			return err
		}
		state = proxyState{Active: true, Files: map[string]proxyFileSnapshot{
			"settings": {Existed: fileExisted, Fields: fields, Containers: containers},
		}}
	} else if _, err := m.claudeSnapshot(state); err != nil {
		return err
	}
	updated, err := applyJSONFieldEdits(data, m.claudeDesiredEdits())
	if err != nil {
		return err
	}
	if newCycle {
		if err := m.stateStore.save(state); err != nil {
			return err
		}
	}
	return writeConfigFileIfChanged(settingsPath, data, updated)
}

func (m *ClaudeManager) DisableProxy() error {
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
		// 旧版本没有字段快照：当前 CLI 文件就是用户要求的原状，不做任何读写。
		return m.stateStore.save(proxyState{Active: false})
	}
	snapshot, err := m.claudeSnapshot(state)
	if err != nil {
		return err
	}
	settingsPath, err := m.settingsPath()
	if err != nil {
		return err
	}
	data, currentExists, err := readConfigFile(settingsPath, []byte("{}\n"))
	if err != nil {
		return err
	}
	edits, err := jsonSnapshotEdits(snapshot.Fields, claudeProxyFieldPaths)
	if err != nil {
		return err
	}
	updated, err := applyJSONFieldEdits(data, edits)
	if err != nil {
		return err
	}
	if !snapshot.Containers["env"] {
		updated, err = pruneJSONContainers(updated, []string{"env"})
		if err != nil {
			return err
		}
	}
	if !snapshot.Existed {
		empty, err := jsonRootIsEmpty(updated)
		if err != nil {
			return err
		}
		if empty {
			if currentExists {
				if err := removeConfigFile(settingsPath); err != nil {
					return err
				}
			}
			return m.stateStore.save(proxyState{Active: false})
		}
	}
	if err := writeConfigFileIfChanged(settingsPath, data, updated); err != nil {
		return err
	}
	return m.stateStore.save(proxyState{Active: false})
}

func (m *ClaudeManager) claudeDesiredEdits() []jsonFieldEdit {
	return []jsonFieldEdit{
		{Path: claudeProxyFieldPaths[0], Raw: strconv.Quote(claudeAuthTokenValue), Present: true},
		{Path: claudeProxyFieldPaths[1], Raw: strconv.Quote(m.baseURL()), Present: true},
	}
}

func (m *ClaudeManager) claudeSnapshot(state proxyState) (proxyFileSnapshot, error) {
	snapshot, ok := state.Files["settings"]
	if !ok {
		return proxyFileSnapshot{}, errors.New("Claude 接入快照缺少 settings 原值")
	}
	if _, err := jsonSnapshotEdits(snapshot.Fields, claudeProxyFieldPaths); err != nil {
		return proxyFileSnapshot{}, err
	}
	return snapshot, nil
}

func (m *ClaudeManager) settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, claudeSettingsDir, claudeSettingsFileName), nil
}

func (m *ClaudeManager) baseURL() string {
	return normalizeRelayBaseURL(m.relayAddr)
}

type claudeSettingsFile struct {
	Env map[string]string `json:"env"`
}
