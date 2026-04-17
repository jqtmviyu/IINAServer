package session

import (
	"context"
	"sync"

	"github.com/jqtmviyu/iinaServer/internal/model"
)

type ActiveSession struct {
	Review    *model.SessionReview
	Cancel    context.CancelFunc
	SessionID string
}

type Manager struct {
	mu     sync.Mutex
	active *ActiveSession
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Replace(next *ActiveSession) (prev *ActiveSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev = m.active
	m.active = next
	return prev
}

func (m *Manager) Current() *ActiveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *Manager) StopCurrent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.Cancel != nil {
		m.active.Cancel()
	}
	m.active = nil
}

func (m *Manager) ClearIfCurrent(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.SessionID == sessionID {
		m.active = nil
	}
}
