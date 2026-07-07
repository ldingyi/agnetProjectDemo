package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ContextMessage 是 agent 可见的上下文消息。它保存模型下一轮运行需要读取的内容，
// 不承担前端聊天记录展示职责。
type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Session 是 agent 上下文会话模型，只保存模型可见上下文。
type Session struct {
	id        string
	createdAt time.Time
	updatedAt time.Time
	filePath  string
	mu        sync.Mutex
	messages  []*ContextMessage
}

// Store 按用户目录管理 agent 上下文文件；每个上下文会话一个 jsonl。
type Store struct {
	dir   string
	mu    sync.Mutex
	cache map[string]*Session
}

type fileHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type contextLine struct {
	Type    string          `json:"type"`
	Context *ContextMessage `json:"context,omitempty"`
	Message *legacyMessage  `json:"message,omitempty"`
}

type legacyMessage struct {
	Role        string `json:"role"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	Payload     string `json:"payload"`
}

const (
	lineTypeHeader              = "agent_session"
	lineTypeLegacyHeader        = "session"
	lineTypeContextMessage      = "context_message"
	lineTypeAgentContextMessage = "agent_context_message"
	lineTypeLegacyMessage       = "message"
	lineTypeLegacyChatMessage   = "chat_message"
)

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent session dir: %w", err)
	}
	return &Store{
		dir:   dir,
		cache: make(map[string]*Session),
	}, nil
}

func (s *Store) Create() (*Session, error) {
	return s.GetOrCreate(uuid.NewString())
}

func (s *Store) GetOrCreate(id string) (*Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return s.Create()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.cache[id]; ok {
		return sess, nil
	}

	filePath := filepath.Join(s.dir, id+".jsonl")
	var sess *Session
	var err error
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		sess, err = createSession(id, filePath)
	} else {
		sess, err = loadSession(filePath)
	}
	if err != nil {
		return nil, err
	}
	s.cache[id] = sess
	return sess, nil
}

func (s *Store) Get(id string) (*Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("agent session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.cache[id]; ok {
		return sess, nil
	}

	filePath := filepath.Join(s.dir, id+".jsonl")
	sess, err := loadSession(filePath)
	if err != nil {
		return nil, err
	}
	s.cache[id] = sess
	return sess, nil
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Messages() []*ContextMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	return cloneMessages(s.messages)
}

func (s *Session) Append(messages ...*ContextMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lines := make([]contextLine, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		lines = append(lines, contextLine{
			Type:    lineTypeContextMessage,
			Context: cloneMessage(msg),
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

	file, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_WRONLY, 0o644)
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

	s.updatedAt = time.Now().UTC()
	for _, line := range lines {
		s.messages = append(s.messages, line.Context)
	}
	if stat, err := os.Stat(s.filePath); err == nil {
		s.updatedAt = stat.ModTime().UTC()
	}
	return nil
}

func createSession(id string, filePath string) (*Session, error) {
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
	return &Session{
		id:        id,
		createdAt: now,
		updatedAt: now,
		filePath:  filePath,
		messages:  make([]*ContextMessage, 0),
	}, nil
}

func loadSession(filePath string) (*Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty agent session file: %s", filePath)
	}

	var header fileHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("bad agent session header: %w", err)
	}
	if header.Type != "" && header.Type != lineTypeHeader && header.Type != lineTypeLegacyHeader {
		return nil, fmt.Errorf("unexpected agent session header type: %s", header.Type)
	}

	sess := &Session{
		id:        header.ID,
		createdAt: header.CreatedAt,
		updatedAt: header.UpdatedAt,
		filePath:  filePath,
		messages:  make([]*ContextMessage, 0),
	}
	if stat, err := os.Stat(filePath); err == nil {
		sess.updatedAt = stat.ModTime().UTC()
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry contextLine
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		switch entry.Type {
		case lineTypeContextMessage, lineTypeAgentContextMessage:
			if entry.Context != nil {
				sess.messages = append(sess.messages, entry.Context)
			}
		case lineTypeLegacyMessage, lineTypeLegacyChatMessage, "":
			if entry.Message != nil {
				sess.messages = append(sess.messages, contextFromLegacyMessage(entry.Message))
			}
		}
		if header.UpdatedAt.IsZero() {
			sess.updatedAt = header.CreatedAt
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sess, nil
}

func contextFromLegacyMessage(message *legacyMessage) *ContextMessage {
	if message == nil {
		return nil
	}
	content := strings.TrimSpace(message.Content)
	payload := strings.TrimSpace(message.Payload)
	if payload != "" {
		if content != "" {
			content += "\n\n"
		}
		content += "以下是历史结构化数据，供后续对话引用：\n" + payload
	}
	return &ContextMessage{
		Role:    message.Role,
		Content: content,
	}
}

func cloneMessages(messages []*ContextMessage) []*ContextMessage {
	result := make([]*ContextMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, cloneMessage(message))
	}
	return result
}

func cloneMessage(message *ContextMessage) *ContextMessage {
	if message == nil {
		return nil
	}
	return &ContextMessage{
		Role:    message.Role,
		Content: message.Content,
	}
}
