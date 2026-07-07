package session

import "testing"

func TestManagerScopesSessionsByUser(t *testing.T) {
	manager := NewManager(t.TempDir())

	first, err := manager.GetOrCreate("user-a", "shared-id")
	if err != nil {
		t.Fatalf("get first user session: %v", err)
	}
	second, err := manager.GetOrCreate("user-b", "shared-id")
	if err != nil {
		t.Fatalf("get second user session: %v", err)
	}
	if first == second {
		t.Fatal("sessions for different users should not share the same instance")
	}

	reloaded, err := manager.GetOrCreate("user-a", "shared-id")
	if err != nil {
		t.Fatalf("reload first user session: %v", err)
	}
	if reloaded != first {
		t.Fatal("manager should reuse cached session store for the same user")
	}
}
