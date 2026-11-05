package infrastructure

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"portico/internal/shared/storage"
)

const proxyStateVersion = 1

// proxyState 只保存一次接入周期内受管字段的原值，不保存 CLI 整文件。
// Files 的 key 是平台内固定的逻辑文件名（settings/config/auth/env）。
type proxyState struct {
	Version int                          `json:"version"`
	Active  bool                         `json:"active"`
	Files   map[string]proxyFileSnapshot `json:"files,omitempty"`
}

type proxyFileSnapshot struct {
	Existed    bool                `json:"existed"`
	Fields     map[string][]string `json:"fields,omitempty"`
	Containers map[string]bool     `json:"containers,omitempty"`
}

type proxyStateStore struct {
	platform string
}

func newProxyStateStore(platform string) proxyStateStore {
	return proxyStateStore{platform: platform}
}

func (s proxyStateStore) path() (string, error) {
	return storage.AppDataPath(filepath.Join("platformproxy", s.platform+".json"))
}

// load 的第二个返回值表示状态文件是否存在。只有 os.ErrNotExist 才表示
// 没有快照；损坏或未知版本必须报错，避免覆盖仍可恢复的原值。
func (s proxyStateStore) load() (proxyState, bool, error) {
	path, err := s.path()
	if err != nil {
		return proxyState{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return proxyState{}, false, nil
		}
		return proxyState{}, false, err
	}
	var state proxyState
	if err := json.Unmarshal(data, &state); err != nil {
		return proxyState{}, false, fmt.Errorf("解析平台接入快照 %s 失败: %w", path, err)
	}
	if state.Version != proxyStateVersion {
		return proxyState{}, false, fmt.Errorf("平台接入快照 %s 的版本 %d 不受支持", path, state.Version)
	}
	if state.Active && len(state.Files) == 0 {
		return proxyState{}, false, fmt.Errorf("平台接入快照 %s 缺少字段原值", path)
	}
	return state, true, nil
}

func (s proxyStateStore) save(state proxyState) error {
	if state.Active && len(state.Files) == 0 {
		return errors.New("激活平台接入快照前必须保存字段原值")
	}
	state.Version = proxyStateVersion
	if !state.Active {
		// 关闭后清除可能包含 API key 的原值，只保留明确的关闭状态。
		state.Files = nil
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	path, err := s.path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWriteFile(path, payload, 0o600)
}
