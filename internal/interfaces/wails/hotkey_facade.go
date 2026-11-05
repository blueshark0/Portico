package wails

import (
	hotkeydomain "portico/internal/hotkeys/domain"
)

// HotkeyRepository 是 facade 对快捷键存储的窄依赖（由
// hotkeys/infrastructure.SQLiteStore 满足）。快捷键读写没有用例逻辑，
// facade 直接依赖存储接口，不再经过逐字转发的 application 层。
type HotkeyRepository interface {
	List() ([]hotkeydomain.Hotkey, error)
	Update(id int, key int, modifier int) error
	Close() error
}

type HotkeyFacade struct {
	repo HotkeyRepository
}

func NewHotkeyFacade(repo HotkeyRepository) *HotkeyFacade {
	return &HotkeyFacade{repo: repo}
}

func (f *HotkeyFacade) GetHotkeys() ([]hotkeydomain.Hotkey, error) {
	return f.repo.List()
}

func (f *HotkeyFacade) UpHotkey(id int, key int, modifier int) error {
	return f.repo.Update(id, key, modifier)
}

func (f *HotkeyFacade) Close() error {
	return f.repo.Close()
}
