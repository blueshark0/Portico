package domain

import "time"

// SessionBinding stores the active provider selected for a platform session.
type SessionBinding struct {
	Platform      string    `json:"platform"`
	SessionID     string    `json:"session_id"`
	ProviderName  string    `json:"provider_name"`
	LastSuccessAt time.Time `json:"last_success_at"`
	CreatedAt     time.Time `json:"created_at"`
}
