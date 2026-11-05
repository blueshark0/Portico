package domain

type Hotkey struct {
	ID        int    `json:"id"`
	KeyCode   uint32 `json:"keycode"`
	Modifiers uint32 `json:"modifiers"`
}
