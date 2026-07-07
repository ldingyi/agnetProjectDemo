package chatthread

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChatThreadPersistsVisibleMessages(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	thread, err := store.Create()
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	if err := thread.Append(
		&Message{Role: "user", Content: "帮我总结 IM", ContentType: "text"},
		&Message{Role: "assistant", Content: "已完成总结", ContentType: "im_chat_summary", Payload: `{"new_offers":[]}`},
	); err != nil {
		t.Fatalf("append messages: %v", err)
	}
	if got := thread.Messages(); len(got) != 2 || got[1].Payload == "" {
		t.Fatalf("visible messages not preserved: %#v", got)
	}
	if title := thread.Info().Title; title != "帮我总结 IM" {
		t.Fatalf("title should come from visible user message, got %q", title)
	}

	reloadedStore, err := NewStore(filepath.Dir(thread.filePath))
	if err != nil {
		t.Fatalf("new reload store: %v", err)
	}
	reloaded, err := reloadedStore.Get(thread.ID())
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	if got := reloaded.Messages(); len(got) != 2 || got[1].Payload == "" {
		t.Fatalf("reloaded visible messages not preserved: %#v", got)
	}
}

func TestLoadThreadReadsLegacyVisibleMessages(t *testing.T) {
	dir := t.TempDir()
	id := "legacy-thread"
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
	legacyLine, err := json.Marshal(messageLine{
		Type:    lineTypeVisibleMessage,
		Message: &Message{Role: "assistant", Content: "展示文本", ContentType: "im_chat_summary", Payload: `{"new_offers":[]}`},
	})
	if err != nil {
		t.Fatalf("marshal legacy line: %v", err)
	}
	if err := os.WriteFile(filePath, append(append(header, '\n'), append(legacyLine, '\n')...), 0o644); err != nil {
		t.Fatalf("write legacy thread: %v", err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	thread, err := store.Get(id)
	if err != nil {
		t.Fatalf("get legacy thread: %v", err)
	}
	if got := thread.Messages(); len(got) != 1 || got[0].Payload == "" {
		t.Fatalf("legacy visible message not loaded: %#v", got)
	}
}
