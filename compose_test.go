package gogenconf_test

import (
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
)

func TestComposeDeterministicLayers(t *testing.T) {
	defaultsText := `config for downloadsd version 1

section service
    host "127.0.0.1"
    port 8080
    timeout "30s"

    section storage
        driver "local"
        path "/var/lib/downloadsd"
    end
end

section network
    routes { default: "192.168.1.1", internal: "10.0.0.1" }
end
`

	sysFrag1Text := `config for downloadsd version 1

section service
    port 9090
end
`

	sysFrag2Text := `config for downloadsd version 1

section service
    section storage
        path "/mnt/storage/downloadsd"
    end
end

section network
    routes { fallback: "10.0.0.254" }
end
`

	userText := `config for downloadsd version 1

section service
    host "0.0.0.0"

    section storage
        max_size "500GB"
    end
end
`

	defDoc, err := gogenconf.ParseNamed(strings.NewReader(defaultsText), "00-defaults.conf")
	if err != nil {
		t.Fatal(err)
	}
	sys1Doc, err := gogenconf.ParseNamed(strings.NewReader(sysFrag1Text), "10-sys-port.conf")
	if err != nil {
		t.Fatal(err)
	}
	sys2Doc, err := gogenconf.ParseNamed(strings.NewReader(sysFrag2Text), "20-sys-storage.conf")
	if err != nil {
		t.Fatal(err)
	}
	userDoc, err := gogenconf.ParseNamed(strings.NewReader(userText), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}

	// Shuffle input order to verify priority-driven deterministic ordering
	layers := []gogenconf.SourceLayer{
		{Name: "user", Document: userDoc, Priority: gogenconf.PriorityUser, Order: 0},
		{Name: "sys2", Document: sys2Doc, Priority: gogenconf.PrioritySystem, Order: 2},
		{Name: "defaults", Document: defDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "sys1", Document: sys1Doc, Priority: gogenconf.PrioritySystem, Order: 1},
	}

	effective, err := gogenconf.Compose(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// 1. Verify scalar overrides and layer precedence
	// host should be user's "0.0.0.0"
	hostExpr := effective.PathValue("service", "host")
	if lit, ok := hostExpr.(gogenconf.Literal); !ok || lit.Value != "0.0.0.0" {
		t.Errorf("host = %v; want 0.0.0.0", hostExpr)
	}
	// Verify provenance: host came from 99-user.conf
	if span := gogenconf.SpanOf(hostExpr); span.Source != "99-user.conf" {
		t.Errorf("host provenance = %q; want 99-user.conf", span.Source)
	}

	// port should be sys1's 9090 (overrode default 8080)
	portExpr := effective.PathValue("service", "port")
	if lit, ok := portExpr.(gogenconf.Literal); !ok || lit.Value != "9090" {
		t.Errorf("port = %v; want 9090", portExpr)
	}
	if span := gogenconf.SpanOf(portExpr); span.Source != "10-sys-port.conf" {
		t.Errorf("port provenance = %q; want 10-sys-port.conf", span.Source)
	}

	// timeout was never overridden, should come from defaults
	timeoutExpr := effective.PathValue("service", "timeout")
	if lit, ok := timeoutExpr.(gogenconf.Literal); !ok || lit.Value != "30s" {
		t.Errorf("timeout = %v; want 30s", timeoutExpr)
	}
	if span := gogenconf.SpanOf(timeoutExpr); span.Source != "00-defaults.conf" {
		t.Errorf("timeout provenance = %q; want 00-defaults.conf", span.Source)
	}

	// 2. Nested section composition
	// driver from defaults
	driverExpr := effective.PathValue("service", "storage", "driver")
	if lit, ok := driverExpr.(gogenconf.Literal); !ok || lit.Value != "local" {
		t.Errorf("driver = %v; want local", driverExpr)
	}
	// path from sys2 (overrode defaults)
	pathExpr := effective.PathValue("service", "storage", "path")
	if lit, ok := pathExpr.(gogenconf.Literal); !ok || lit.Value != "/mnt/storage/downloadsd" {
		t.Errorf("path = %v; want /mnt/storage/downloadsd", pathExpr)
	}
	// max_size added by user
	maxExpr := effective.PathValue("service", "storage", "max_size")
	if lit, ok := maxExpr.(gogenconf.Literal); !ok || lit.Value != "500GB" {
		t.Errorf("max_size = %v; want 500GB", maxExpr)
	}

	// 3. Keyed map merge
	routesExpr := effective.PathValue("network", "routes")
	mp, ok := routesExpr.(gogenconf.Map)
	if !ok || len(mp.Entries) != 3 {
		t.Fatalf("routes map = %v; want 3 merged entries", routesExpr)
	}

	routesMap := map[string]string{}
	for _, en := range mp.Entries {
		routesMap[en.Key] = en.Value.(gogenconf.Literal).Value
	}
	if routesMap["default"] != "192.168.1.1" || routesMap["internal"] != "10.0.0.1" || routesMap["fallback"] != "10.0.0.254" {
		t.Errorf("merged routes mismatch: %+v", routesMap)
	}

	// 4. Verify non-mutating source isolation
	// Original defDoc host should still be 127.0.0.1
	defHost := defDoc.PathValue("service", "host")
	if lit, ok := defHost.(gogenconf.Literal); !ok || lit.Value != "127.0.0.1" {
		t.Fatalf("defaults document was mutated! got %v", defHost)
	}
}

func TestComposeCompatibilityCheck(t *testing.T) {
	l1 := gogenconf.SourceLayer{
		Name:     "sys",
		Document: &gogenconf.Document{Identity: "myapp", Version: 1, HasVersion: true},
		Priority: gogenconf.PrioritySystem,
	}
	l2 := gogenconf.SourceLayer{
		Name:     "user",
		Document: &gogenconf.Document{Identity: "otherapp", Version: 1, HasVersion: true},
		Priority: gogenconf.PriorityUser,
	}

	_, err := gogenconf.Compose([]gogenconf.SourceLayer{l1, l2}, gogenconf.CompositionOptions{
		ExpectedIdentity: "myapp",
		MaxVersion:       1,
	})
	if err == nil {
		t.Fatal("expected error on identity mismatch across layers, got nil")
	}
	if !strings.Contains(err.Error(), "compatibility error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComposeFieldDeletion(t *testing.T) {
	baseText := `config for myapp version 1
section server
    host "127.0.0.1"
    temp_flag "remove-me"
end
`
	overrideText := `config for myapp version 1
section server
    temp_flag
end
`

	baseDoc, err := gogenconf.ParseNamed(strings.NewReader(baseText), "base.conf")
	if err != nil {
		t.Fatal(err)
	}
	overrideDoc, err := gogenconf.ParseNamed(strings.NewReader(overrideText), "override.conf")
	if err != nil {
		t.Fatal(err)
	}

	layers := []gogenconf.SourceLayer{
		{Name: "base", Document: baseDoc, Priority: gogenconf.PriorityDefault},
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser},
	}

	effective, err := gogenconf.Compose(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "myapp",
		MaxVersion:       1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if effective.PathValue("server", "temp_flag") != nil {
		t.Errorf("expected temp_flag to be deleted, got %v", effective.PathValue("server", "temp_flag"))
	}
	if lit, ok := effective.PathValue("server", "host").(gogenconf.Literal); !ok || lit.Value != "127.0.0.1" {
		t.Errorf("host was unexpectedly altered: %v", effective.PathValue("server", "host"))
	}
}
