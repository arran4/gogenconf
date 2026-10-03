package testfixture

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/address/internal/configmodel"
)

func TestGeneratedTypesBindEagerKeysAndDeferredContent(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	contentPath := filepath.Join(dir, "large")
	key := []byte(" \x00123456789abcd\n")
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIXTURE_KEY", keyPath)
	t.Setenv("FIXTURE_CONTENT", contentPath)
	doc, err := m.Parse(strings.NewReader("config_version 2\nsection service\n key from_file(from_env(FIXTURE_KEY))\n content from_file(from_env(FIXTURE_CONTENT))\nend\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := m.NewStandardRegistry()
	c, err := Resolve(context.Background(), doc, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	// These assignments prove generated API types at compile time.
	var label string = c.Service.Label
	var actualKey []byte = c.Service.Key
	var content m.Provider[[]byte] = c.Service.Content
	if label != "default" || !bytes.Equal(actualKey, key) {
		t.Fatal("incorrect eager binding")
	}
	// Binding succeeded while the provider's file did not exist.
	if _, err := content.Resolve(context.Background()); err == nil {
		t.Fatal("missing content accepted")
	}
	if err := os.WriteFile(contentPath, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := content.Resolve(context.Background())
	if err != nil || string(got) != "first" {
		t.Fatalf("provider = %q, %v", got, err)
	}
	// Snapshot the declaration: subsequent document edits do not retarget it.
	doc.Section("service").Set("content", m.Literal{Value: "changed declaration"})
	if err := os.WriteFile(contentPath, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = content.Resolve(context.Background())
	if err != nil || string(got) != "second" {
		t.Fatalf("reload = %q, %v", got, err)
	}
}

func TestGeneratedRequiredAndSensitiveErrors(t *testing.T) {
	r, _ := m.NewStandardRegistry()
	d := &m.Document{Version: 2, HasVersion: true}
	if _, err := Resolve(context.Background(), d, r, nil); err == nil {
		t.Fatal("required key missing")
	}
	s := &m.Section{Name: "service"}
	d.AddSection(s)
	s.Set("key", m.Call{Name: "leaky"})
	_ = r.Register("leaky", []byte(nil), func(context.Context, *m.Registry, []m.Expr) (any, error) {
		return nil, fmt.Errorf("DISTINCTIVE_SECRET")
	})
	if _, err := Resolve(context.Background(), d, r, nil); err == nil || strings.Contains(err.Error(), "DISTINCTIVE_SECRET") {
		t.Fatalf("error = %v", err)
	}
}

// A cache/content implementation can adapt Provider without seeing an AST.
// This deliberately small local adapter proves the boundary without duplicating
// dynamic-content's invalidation, concurrency or storage lifecycle.
type contentAdapter[T any] struct{ provider m.Provider[T] }

func (a contentAdapter[T]) Data() (*T, error) {
	v, err := a.provider.Resolve(context.Background())
	if err != nil {
		return nil, err
	}
	return &v, nil
}
func TestProviderAdapter(t *testing.T) {
	a := contentAdapter[string]{provider: m.ProviderFunc[string](func(context.Context) (string, error) { return "content", nil })}
	v, err := a.Data()
	if err != nil || *v != "content" {
		t.Fatal("adapter failed")
	}
}
