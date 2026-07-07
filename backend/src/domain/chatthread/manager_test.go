package chatthread

import "testing"

func TestManagerScopesThreadsByUser(t *testing.T) {
	manager := NewManager(t.TempDir())

	first, err := manager.GetOrCreate("user-a", "shared-id")
	if err != nil {
		t.Fatalf("get first user thread: %v", err)
	}
	second, err := manager.GetOrCreate("user-b", "shared-id")
	if err != nil {
		t.Fatalf("get second user thread: %v", err)
	}
	if first == second {
		t.Fatal("threads for different users should not share the same instance")
	}

	reloaded, err := manager.Get("user-a", "shared-id")
	if err != nil {
		t.Fatalf("reload first user thread: %v", err)
	}
	if reloaded != first {
		t.Fatal("manager should reuse cached thread store for the same user")
	}
}
