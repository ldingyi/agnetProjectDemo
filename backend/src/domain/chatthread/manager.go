package chatthread

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// Manager 按用户管理可见聊天会话存储。
type Manager struct {
	rootDir string
	mu      sync.Mutex
	stores  map[string]*Store
}

func NewManager(rootDir string) *Manager {
	return &Manager{
		rootDir: rootDir,
		stores:  make(map[string]*Store),
	}
}

func (m *Manager) Create(userID string) (*ChatThread, error) {
	store, err := m.store(userID)
	if err != nil {
		return nil, err
	}
	return store.Create()
}

func (m *Manager) GetOrCreate(userID string, threadID string) (*ChatThread, error) {
	store, err := m.store(userID)
	if err != nil {
		return nil, err
	}
	return store.GetOrCreate(threadID)
}

func (m *Manager) Get(userID string, threadID string) (*ChatThread, error) {
	store, err := m.store(userID)
	if err != nil {
		return nil, err
	}
	return store.Get(threadID)
}

func (m *Manager) List(userID string) ([]Info, error) {
	store, err := m.store(userID)
	if err != nil {
		return nil, err
	}
	return store.List()
}

func (m *Manager) store(userID string) (*Store, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if store, ok := m.stores[userID]; ok {
		return store, nil
	}
	store, err := NewStore(filepath.Join(m.rootDir, userID, "chat_threads"))
	if err != nil {
		return nil, err
	}
	m.stores[userID] = store
	return store, nil
}
