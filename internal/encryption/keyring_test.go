package encryption

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
)

func init() {
	// Use mock keyring backend for tests (no real OS keychain access).
	keyring.MockInit()
}

func TestKeyringProvider_SetAndGet(t *testing.T) {
	p := NewKeyringProvider("/tmp/test.db")

	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if err := p.SetKey(key); err != nil {
		t.Fatalf("SetKey: %v", err)
	}

	got, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}

	if len(got) != KeySize {
		t.Errorf("key size = %d, want %d", len(got), KeySize)
	}
	for i := range key {
		if got[i] != key[i] {
			t.Fatalf("key mismatch at byte %d", i)
		}
	}
}

func TestKeyringProvider_NotFound(t *testing.T) {
	p := NewKeyringProvider("/tmp/nonexistent.db")

	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("GetKey should fail when no key is stored")
	}
}

func TestKeyringProvider_MultipleDBs(t *testing.T) {
	p1 := NewKeyringProvider("/tmp/db1.db")
	p2 := NewKeyringProvider("/tmp/db2.db")

	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if err := p1.SetKey(key1); err != nil {
		t.Fatalf("SetKey p1: %v", err)
	}
	if err := p2.SetKey(key2); err != nil {
		t.Fatalf("SetKey p2: %v", err)
	}

	got1, err := p1.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey p1: %v", err)
	}
	got2, err := p2.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey p2: %v", err)
	}

	// Keys should be different
	same := true
	for i := range got1 {
		if got1[i] != got2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("keys for different DBs should be different")
	}

	// Verify each key matches what was set
	for i := range key1 {
		if got1[i] != key1[i] {
			t.Fatalf("db1 key mismatch at byte %d", i)
		}
	}
	for i := range key2 {
		if got2[i] != key2[i] {
			t.Fatalf("db2 key mismatch at byte %d", i)
		}
	}
}

func TestKeyringProvider_DeleteKey(t *testing.T) {
	p := NewKeyringProvider("/tmp/delete-test.db")

	key, err := GenerateKey(); if err != nil { t.Fatalf("GenerateKey: %v", err) }
	if err := p.SetKey(key); err != nil {
		t.Fatalf("SetKey: %v", err)
	}

	if err := p.DeleteKey(); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}

	_, err = p.GetKey(context.Background())
	if err == nil {
		t.Fatal("GetKey should fail after DeleteKey")
	}
}

func TestKeyringProvider_DeleteKey_NotFound(t *testing.T) {
	p := NewKeyringProvider("/tmp/never-stored.db")

	// Should not error when deleting a non-existent key
	if err := p.DeleteKey(); err != nil {
		t.Fatalf("DeleteKey on non-existent key: %v", err)
	}
}

func TestKeyringProvider_Name(t *testing.T) {
	p := NewKeyringProvider("/tmp/test.db")
	if p.Name() != "keyring" {
		t.Errorf("Name() = %q, want %q", p.Name(), "keyring")
	}
}
