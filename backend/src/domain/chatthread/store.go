package chatthread

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Info struct {
	ID        string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message 是用户可见的聊天消息，只表达前端聊天记录需要展示的内容。
type Message struct {
	Role        string `json:"role"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	Payload     string `json:"payload,omitempty"`
}

// ChatThread 是用户可见的聊天会话模型，等价于传统 IM 产品里的会话。
type ChatThread struct {
	id        string
	createdAt time.Time
	updatedAt time.Time
	filePath  string
	mu        sync.Mutex
	messages  []*Message
}

// Store 按用户目录管理聊天会话文件；每个会话一个 jsonl，便于追加写入和本地调试。
type Store struct {
	dir   string
	mu    sync.Mutex
	cache map[string]*ChatThread
}

type fileHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type messageLine struct {
	Type    string   `json:"type"`
	Message *Message `json:"message,omitempty"`
}

const (
	lineTypeHeader         = "chat_thread"
	lineTypeLegacyHeader   = "session"
	lineTypeMessage        = "message"
	lineTypeVisibleMessage = "chat_message"
)

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create chat thread dir: %w", err)
	}
	return &Store{
		dir:   dir,
		cache: make(map[string]*ChatThread),
	}, nil
}

func (s *Store) Create() (*ChatThread, error) {
	return s.GetOrCreate(uuid.NewString())
}

func (s *Store) GetOrCreate(id string) (*ChatThread, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return s.Create()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if thread, ok := s.cache[id]; ok {
		return thread, nil
	}

	filePath := filepath.Join(s.dir, id+".jsonl")
	var thread *ChatThread
	var err error
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		thread, err = createThread(id, filePath)
	} else {
		thread, err = loadThread(filePath)
	}
	if err != nil {
		return nil, err
	}
	s.cache[id] = thread
	return thread, nil
}

func (s *Store) Get(id string) (*ChatThread, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("chat thread id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if thread, ok := s.cache[id]; ok {
		return thread, nil
	}

	filePath := filepath.Join(s.dir, id+".jsonl")
	thread, err := loadThread(filePath)
	if err != nil {
		return nil, err
	}
	s.cache[id] = thread
	return thread, nil
}

func (s *Store) List() ([]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	infos := make([]Info, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		thread, err := s.Get(strings.TrimSuffix(entry.Name(), ".jsonl"))
		if err != nil {
			continue
		}
		infos = append(infos, thread.Info())
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt.After(infos[j].UpdatedAt)
	})
	return infos, nil
}

func (t *ChatThread) ID() string {
	return t.id
}

func (t *ChatThread) Info() Info {
	t.mu.Lock()
	defer t.mu.Unlock()

	return Info{
		ID:        t.id,
		Title:     t.titleLocked(),
		CreatedAt: t.createdAt,
		UpdatedAt: t.updatedAt,
	}
}

func (t *ChatThread) Messages() []*Message {
	t.mu.Lock()
	defer t.mu.Unlock()

	return cloneMessages(t.messages)
}

func (t *ChatThread) Append(messages ...*Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	lines := make([]messageLine, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		lines = append(lines, messageLine{
			Type:    lineTypeMessage,
			Message: cloneMessage(msg),
		})
	}
	if len(lines) == 0 {
		return nil
	}

	encoded := make([][]byte, 0, len(lines))
	for _, line := range lines {
		data, err := json.Marshal(line)
		if err != nil {
			return err
		}
		encoded = append(encoded, data)
	}

	file, err := os.OpenFile(t.filePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, data := range encoded {
		if _, err = fmt.Fprintf(file, "%s\n", data); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}

	t.updatedAt = time.Now().UTC()
	for _, line := range lines {
		t.messages = append(t.messages, line.Message)
	}
	if stat, err := os.Stat(t.filePath); err == nil {
		t.updatedAt = stat.ModTime().UTC()
	}
	return nil
}

func (t *ChatThread) titleLocked() string {
	for _, msg := range t.messages {
		if msg != nil && msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			runes := []rune(strings.TrimSpace(msg.Content))
			if len(runes) > 32 {
				return string(runes[:32]) + "..."
			}
			return string(runes)
		}
	}
	return "New Session"
}

func createThread(id string, filePath string) (*ChatThread, error) {
	now := time.Now().UTC()
	header := fileHeader{
		Type:      lineTypeHeader,
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
	}
	data, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return &ChatThread{
		id:        id,
		createdAt: now,
		updatedAt: now,
		filePath:  filePath,
		messages:  make([]*Message, 0),
	}, nil
}

func loadThread(filePath string) (*ChatThread, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty chat thread file: %s", filePath)
	}

	var header fileHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("bad chat thread header: %w", err)
	}
	if header.Type != "" && header.Type != lineTypeHeader && header.Type != lineTypeLegacyHeader {
		return nil, fmt.Errorf("unexpected chat thread header type: %s", header.Type)
	}

	thread := &ChatThread{
		id:        header.ID,
		createdAt: header.CreatedAt,
		updatedAt: header.UpdatedAt,
		filePath:  filePath,
		messages:  make([]*Message, 0),
	}
	if stat, err := os.Stat(filePath); err == nil {
		thread.updatedAt = stat.ModTime().UTC()
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry messageLine
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Message == nil {
			continue
		}
		if entry.Type == lineTypeMessage || entry.Type == lineTypeVisibleMessage || entry.Type == "" {
			thread.messages = append(thread.messages, entry.Message)
		}
		if header.UpdatedAt.IsZero() {
			thread.updatedAt = header.CreatedAt
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return thread, nil
}

func cloneMessages(messages []*Message) []*Message {
	result := make([]*Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, cloneMessage(message))
	}
	return result
}

func cloneMessage(message *Message) *Message {
	if message == nil {
		return nil
	}
	return &Message{
		Role:        message.Role,
		Content:     message.Content,
		ContentType: message.ContentType,
		Payload:     message.Payload,
	}
}
