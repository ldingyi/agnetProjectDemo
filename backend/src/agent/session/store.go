package session

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

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

type Info struct {
	ID        string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Session struct {
	id        string
	createdAt time.Time
	updatedAt time.Time
	filePath  string
	mu        sync.Mutex
	messages  []*schema.Message
}

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

type messageLine struct {
	Type    string          `json:"type"`
	Message *schema.Message `json:"message"`
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
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
		return nil, fmt.Errorf("session id is required")
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
		sess, err := s.Get(strings.TrimSuffix(entry.Name(), ".jsonl"))
		if err != nil {
			continue
		}
		infos = append(infos, sess.Info())
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt.After(infos[j].UpdatedAt)
	})
	return infos, nil
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Info{
		ID:        s.id,
		Title:     s.titleLocked(),
		CreatedAt: s.createdAt,
		UpdatedAt: s.updatedAt,
	}
}

func (s *Session) Messages() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := make([]*schema.Message, len(s.messages))
	copy(messages, s.messages)
	return messages
}

func (s *Session) Append(msg *schema.Message) error {
	if msg == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.messages = append(s.messages, msg)
	s.updatedAt = now

	line := messageLine{
		Type:    "message",
		Message: msg,
	}
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	if _, err = fmt.Fprintf(file, "%s\n", data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if stat, err := os.Stat(s.filePath); err == nil {
		s.updatedAt = stat.ModTime().UTC()
	}
	return nil
}

func (s *Session) titleLocked() string {
	for _, msg := range s.messages {
		if msg != nil && msg.Role == schema.User && strings.TrimSpace(msg.Content) != "" {
			runes := []rune(strings.TrimSpace(msg.Content))
			if len(runes) > 32 {
				return string(runes[:32]) + "..."
			}
			return string(runes)
		}
	}
	return "New Session"
}

func createSession(id string, filePath string) (*Session, error) {
	now := time.Now().UTC()
	header := fileHeader{
		Type:      "session",
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
		messages:  make([]*schema.Message, 0),
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
		return nil, fmt.Errorf("empty session file: %s", filePath)
	}

	var header fileHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("bad session header: %w", err)
	}

	sess := &Session{
		id:        header.ID,
		createdAt: header.CreatedAt,
		updatedAt: header.UpdatedAt,
		filePath:  filePath,
		messages:  make([]*schema.Message, 0),
	}
	if stat, err := os.Stat(filePath); err == nil {
		sess.updatedAt = stat.ModTime().UTC()
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
		sess.messages = append(sess.messages, entry.Message)
		if header.UpdatedAt.IsZero() {
			sess.updatedAt = header.CreatedAt
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sess, nil
}
