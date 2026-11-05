package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	platformproxydomain "portico/internal/platformproxy/domain"
)

const (
	geminiSettingsDir      = ".gemini"
	geminiSettingsFileName = ".env"
	geminiAPIKeyValue      = "portico"
)

var geminiProxyFieldKeys = []string{"GEMINI_API_KEY", "GOOGLE_GEMINI_BASE_URL"}

type GeminiManager struct {
	relayAddr  string
	stateStore proxyStateStore
	mu         sync.RWMutex
}

func NewGeminiManager(relayAddr string) *GeminiManager {
	return &GeminiManager{relayAddr: relayAddr, stateStore: newProxyStateStore("gemini")}
}

func (m *GeminiManager) ProxyStatus() (platformproxydomain.Status, error) {
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
	envMap, err := envEffectiveValues(data, geminiProxyFieldKeys)
	if err != nil {
		return status, err
	}
	status.Enabled = isManagedPlaceholderToken(envMap["GEMINI_API_KEY"], geminiAPIKeyValue) &&
		strings.EqualFold(envMap["GOOGLE_GEMINI_BASE_URL"], m.baseURL())
	return status, nil
}

func (m *GeminiManager) EnableProxy() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	settingsPath, err := m.settingsPath()
	if err != nil {
		return err
	}
	data, fileExisted, err := readConfigFile(settingsPath, nil)
	if err != nil {
		return err
	}
	state, stateExists, err := m.stateStore.load()
	if err != nil {
		return err
	}
	newCycle := !stateExists || !state.Active
	if newCycle {
		fields, err := readEnvFieldValues(data, geminiProxyFieldKeys)
		if err != nil {
			return err
		}
		state = proxyState{Active: true, Files: map[string]proxyFileSnapshot{
			"env": {Existed: fileExisted, Fields: fields},
		}}
	} else if _, err := m.geminiSnapshot(state); err != nil {
		return err
	}
	updated, err := m.applyGeminiValues(data)
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

func (m *GeminiManager) DisableProxy() error {
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
	snapshot, err := m.geminiSnapshot(state)
	if err != nil {
		return err
	}
	settingsPath, err := m.settingsPath()
	if err != nil {
		return err
	}
	data, currentExists, err := readConfigFile(settingsPath, nil)
	if err != nil {
		return err
	}
	updated := data
	for _, key := range geminiProxyFieldKeys {
		updated, err = restoreEnvField(updated, key, snapshot.Fields[key])
		if err != nil {
			return err
		}
	}
	if !snapshot.Existed && len(updated) == 0 {
		if currentExists {
			if err := removeConfigFile(settingsPath); err != nil {
				return err
			}
		}
		return m.stateStore.save(proxyState{Active: false})
	}
	if err := writeConfigFileIfChanged(settingsPath, data, updated); err != nil {
		return err
	}
	return m.stateStore.save(proxyState{Active: false})
}

func (m *GeminiManager) applyGeminiValues(data []byte) ([]byte, error) {
	updated, err := setEnvField(data, geminiProxyFieldKeys[0], geminiAPIKeyValue)
	if err != nil {
		return nil, err
	}
	return setEnvField(updated, geminiProxyFieldKeys[1], m.baseURL())
}

func (m *GeminiManager) geminiSnapshot(state proxyState) (proxyFileSnapshot, error) {
	snapshot, ok := state.Files["env"]
	if !ok {
		return proxyFileSnapshot{}, errors.New("Gemini 接入快照缺少 .env 原值")
	}
	for _, key := range geminiProxyFieldKeys {
		for _, line := range snapshot.Fields[key] {
			assignments, err := findEnvAssignments([]string{line}, key)
			if err != nil || len(assignments) != 1 {
				return proxyFileSnapshot{}, errors.New("Gemini 接入快照含有无效的原始赋值")
			}
		}
	}
	return snapshot, nil
}

func (m *GeminiManager) settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, geminiSettingsDir, geminiSettingsFileName), nil
}

func (m *GeminiManager) baseURL() string {
	return normalizeRelayBaseURL(m.relayAddr) + "/gemini"
}
