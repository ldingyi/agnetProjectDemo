package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionPersistsAgentContextMessages(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	sess, err := store.Create()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := sess.Append(
		&ContextMessage{Role: "user", Content: "帮我总结 IM"},
		&ContextMessage{Role: "assistant", Content: "模型可见的总结文本"},
	); err != nil {
		t.Fatalf("append context: %v", err)
	}
	if got := sess.Messages(); len(got) != 2 || got[1].Content != "模型可见的总结文本" {
		t.Fatalf("context messages not preserved: %#v", got)
	}

	reloadedStore, err := NewStore(filepath.Dir(sess.filePath))
	if err != nil {
		t.Fatalf("new reload store: %v", err)
	}
	reloaded, err := reloadedStore.Get(sess.ID())
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if got := reloaded.Messages(); len(got) != 2 || got[1].Content != "模型可见的总结文本" {
		t.Fatalf("reloaded context messages not preserved: %#v", got)
	}
}

func TestLoadSessionMigratesLegacyMessagesToContext(t *testing.T) {
	dir := t.TempDir()
	id := "legacy-session"
	filePath := filepath.Join(dir, id+".jsonl")
	header, err := json.Marshal(fileHeader{
		Type:      lineTypeLegacyHeader,
		ID:        id,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	legacyLine, err := json.Marshal(contextLine{
		Type: lineTypeLegacyMessage,
		Message: &legacyMessage{
			Role:        "assistant",
			Content:     "展示文本",
			ContentType: "im_chat_summary",
			Payload:     `{"conversation_summaries":{"agreed":[]}}`,
		},
	})
	if err != nil {
		t.Fatalf("marshal legacy line: %v", err)
	}
	if err := os.WriteFile(filePath, append(append(header, '\n'), append(legacyLine, '\n')...), 0o644); err != nil {
		t.Fatalf("write legacy session: %v", err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, err := store.Get(id)
	if err != nil {
		t.Fatalf("get legacy session: %v", err)
	}

	contexts := sess.Messages()
	if len(contexts) != 1 || !strings.Contains(contexts[0].Content, "历史结构化数据") {
		t.Fatalf("legacy context message not migrated: %#v", contexts)
	}
}
