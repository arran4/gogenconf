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

func TestComposeDuplicatePriorityOrderRejection(t *testing.T) {
	doc1 := &gogenconf.Document{Identity: "test", Version: 1, HasVersion: true}
	doc2 := &gogenconf.Document{Identity: "test", Version: 1, HasVersion: true}

	layers := []gogenconf.SourceLayer{
		{Name: "layer1.conf", Document: doc1, Priority: gogenconf.PrioritySystem, Order: 1},
		{Name: "layer2.conf", Document: doc2, Priority: gogenconf.PrioritySystem, Order: 1},
	}

	_, err := gogenconf.Compose(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "test",
		MaxVersion:       1,
	})
	if err == nil {
		t.Fatal("expected error on duplicate (Priority, Order) layers, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate layer priority/order") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComposeExplicitMutationOperators(t *testing.T) {
	baseText := `config for app version 1
section service
    tags [ "core", "api" ]
    port 8080
    settings { env: "prod", debug: "false" }
    old_key "to-be-removed"
end
`
	overrideText := `config for app version 1
section service
    [+] tags "auth"
    [=] port 9090
    [~] settings { debug: "true", extra: "enabled" }
    [-] old_key
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
		{Name: "base", Document: baseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}

	var warnings []string
	effective, err := gogenconf.Compose(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "app",
		MaxVersion:       1,
		WarnCollector: func(w gogenconf.CompositionWarning) {
			warnings = append(warnings, w.Message)
		},
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	// 1. [+] appended "auth" to tags list
	tagsVal := effective.PathValue("service", "tags")
	tagsList, ok := tagsVal.(gogenconf.List)
	if !ok || len(tagsList.Items) != 3 {
		t.Fatalf("tags = %v; want 3 items", tagsVal)
	}
	if tagsList.Items[2].(gogenconf.Literal).Value != "auth" {
		t.Errorf("expected tags[2] == 'auth', got %v", tagsList.Items[2])
	}

	// 2. [=] replaced port with 9090
	portVal := effective.PathValue("service", "port")
	if lit, ok := portVal.(gogenconf.Literal); !ok || lit.Value != "9090" {
		t.Errorf("port = %v; want 9090", portVal)
	}

	// 3. [~] mutate / inherit settings map
	settingsVal := effective.PathValue("service", "settings")
	settingsMap, ok := settingsVal.(gogenconf.Map)
	if !ok || len(settingsMap.Entries) != 3 {
		t.Fatalf("settings = %v; want 3 entries", settingsVal)
	}
	resMap := map[string]string{}
	for _, en := range settingsMap.Entries {
		resMap[en.Key] = en.Value.(gogenconf.Literal).Value
	}
	if resMap["env"] != "prod" || resMap["debug"] != "true" || resMap["extra"] != "enabled" {
		t.Errorf("settings map mismatch: %+v", resMap)
	}

	// 4. [-] deleted old_key
	if effective.PathValue("service", "old_key") != nil {
		t.Errorf("expected old_key to be deleted, got %v", effective.PathValue("service", "old_key"))
	}
}

func TestComposeMissingTargetWarnings(t *testing.T) {
	overrideText := `config for app version 1
section service
    [~] nonexistent_map { key: "val" }
    [-] missing_key
end
`
	overrideDoc, err := gogenconf.ParseNamed(strings.NewReader(overrideText), "override.conf")
	if err != nil {
		t.Fatal(err)
	}

	var warnings []gogenconf.CompositionWarning
	effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "app",
		MaxVersion:       1,
		WarnCollector: func(w gogenconf.CompositionWarning) {
			warnings = append(warnings, w)
		},
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// Expect 2 warnings: one for [~] missing target, one for [-] missing target
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Stage != gogenconf.StageCompose || warnings[0].Operation != "[~]" || warnings[0].Path != "service.nonexistent_map" {
		t.Errorf("warning 0 mismatch: %+v", warnings[0])
	}
	if warnings[1].Stage != gogenconf.StageCompose || warnings[1].Operation != "[-]" || warnings[1].Path != "service.missing_key" {
		t.Errorf("warning 1 mismatch: %+v", warnings[1])
	}

	// For missing [~] target, value should have been constructed
	val := effective.PathValue("service", "nonexistent_map")
	if val == nil {
		t.Error("expected nonexistent_map to be constructed for missing [~] target")
	}
}

func TestMutationParseAndFormatRoundTrip(t *testing.T) {
	input := `config for app version 1

section service
    [+] tags "new-tag"
    [=] port 8443
    [~] settings { env: "prod" }
    [-] deprecated_field
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "input.conf")
	if err != nil {
		t.Fatalf("ParseNamed failed: %v", err)
	}

	sec := doc.Section("service")
	if sec == nil {
		t.Fatal("missing service section")
	}

	e, _, ok := sec.Entry("tags")
	if !ok || e.Mutation != gogenconf.MutationAdd {
		t.Errorf("tags mutation = %v; want MutationAdd", e.Mutation)
	}

	e, _, ok = sec.Entry("port")
	if !ok || e.Mutation != gogenconf.MutationReplace {
		t.Errorf("port mutation = %v; want MutationReplace", e.Mutation)
	}

	e, _, ok = sec.Entry("settings")
	if !ok || e.Mutation != gogenconf.MutationMutate {
		t.Errorf("settings mutation = %v; want MutationMutate", e.Mutation)
	}

	e, _, ok = sec.Entry("deprecated_field")
	if !ok || e.Mutation != gogenconf.MutationDelete {
		t.Errorf("deprecated_field mutation = %v; want MutationDelete", e.Mutation)
	}

	formatted := gogenconf.Format(doc)
	doc2, err := gogenconf.ParseNamed(strings.NewReader(formatted), "formatted.conf")
	if err != nil {
		t.Fatalf("re-parsing formatted doc failed: %v", err)
	}

	formatted2 := gogenconf.Format(doc2)
	if formatted != formatted2 {
		t.Errorf("formatting round-trip mismatch:\n%s\nvs\n%s", formatted, formatted2)
	}
}

func TestComposeObjectAndCompactPathMutations(t *testing.T) {
	baseText := `config for app version 1

server {
    host "127.0.0.1"
    port 8080
    tls {
        enabled "true"
    }
}

a b c {
    target linux
    arch amd64
}

storage {
    driver "s3"
}
`

	overrideText := `config for app version 1

[~] server {
    port 9090
    tls {
        cert "/path/to/cert"
    }
}

[=] a b c {
    target darwin
}

[-] storage {
}

[+] monitor {
    interval 10s
}
`

	baseDoc, err := gogenconf.ParseNamed(strings.NewReader(baseText), "base.conf")
	if err != nil {
		t.Fatal(err)
	}
	overrideDoc, err := gogenconf.ParseNamed(strings.NewReader(overrideText), "override.conf")
	if err != nil {
		t.Fatal(err)
	}

	var warnings []gogenconf.CompositionWarning
	effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "base", Document: baseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "app",
		MaxVersion:       1,
		WarnCollector: func(w gogenconf.CompositionWarning) {
			warnings = append(warnings, w)
		},
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", warnings)
	}

	// 1. [~] server: host preserved from base, port updated to 9090, tls.enabled preserved, tls.cert added
	hostVal := effective.PathValue("server", "host")
	if lit, ok := hostVal.(gogenconf.Literal); !ok || lit.Value != "127.0.0.1" {
		t.Errorf("server.host = %v; want 127.0.0.1", hostVal)
	}
	portVal := effective.PathValue("server", "port")
	if lit, ok := portVal.(gogenconf.Literal); !ok || lit.Value != "9090" {
		t.Errorf("server.port = %v; want 9090", portVal)
	}
	tlsEnabled := effective.PathValue("server", "tls", "enabled")
	if lit, ok := tlsEnabled.(gogenconf.Literal); !ok || lit.Value != "true" {
		t.Errorf("server.tls.enabled = %v; want true", tlsEnabled)
	}
	tlsCert := effective.PathValue("server", "tls", "cert")
	if lit, ok := tlsCert.(gogenconf.Literal); !ok || lit.Value != "/path/to/cert" {
		t.Errorf("server.tls.cert = %v; want /path/to/cert", tlsCert)
	}

	// 2. [=] a b c: replaced c block, target is darwin, arch amd64 was deleted/not preserved
	targetVal := effective.PathValue("a", "b", "c", "target")
	if lit, ok := targetVal.(gogenconf.Literal); !ok || lit.Value != "darwin" {
		t.Errorf("a.b.c.target = %v; want darwin", targetVal)
	}
	archVal := effective.PathValue("a", "b", "c", "arch")
	if archVal != nil {
		t.Errorf("a.b.c.arch should have been replaced, got %v", archVal)
	}

	// 3. [-] storage: section deleted
	if effective.Section("storage") != nil {
		t.Errorf("storage section should have been deleted")
	}

	// 4. [+] monitor: new section added
	intervalVal := effective.PathValue("monitor", "interval")
	if lit, ok := intervalVal.(gogenconf.Literal); !ok || lit.Value != "10s" {
		t.Errorf("monitor.interval = %v; want 10s", intervalVal)
	}
}

func TestComposeAllTypesMutationsMatrix(t *testing.T) {
	baseText := `config for matrix version 1

section data
    scalar_a "val_a"
    scalar_b "val_b"
    list_a [ "x", "y" ]
    map_a { k1: "v1", k2: "v2" }
    
    section sub_a
        f1 "original"
    end
end
`

	overrideText := `config for matrix version 1

section data
    [=] scalar_a "replaced_a"
    [-] scalar_b
    [+] list_a [ "z" ]
    [~] map_a { k2: "updated_v2", k3: "new_v3" }
    
    [=] sub_a {
        f2 "fresh"
    }
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

	effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "base", Document: baseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "matrix",
		MaxVersion:       1,
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// Scalar [=]
	sa := effective.PathValue("data", "scalar_a")
	if lit, ok := sa.(gogenconf.Literal); !ok || lit.Value != "replaced_a" {
		t.Errorf("scalar_a = %v; want replaced_a", sa)
	}

	// Scalar [-]
	sb := effective.PathValue("data", "scalar_b")
	if sb != nil {
		t.Errorf("scalar_b should have been deleted, got %v", sb)
	}

	// List [+]
	la := effective.PathValue("data", "list_a")
	listObj, ok := la.(gogenconf.List)
	if !ok || len(listObj.Items) != 3 {
		t.Fatalf("list_a = %v; want 3 items", la)
	}
	if listObj.Items[2].(gogenconf.Literal).Value != "z" {
		t.Errorf("list_a[2] = %v; want z", listObj.Items[2])
	}

	// Map [~]
	ma := effective.PathValue("data", "map_a")
	mapObj, ok := ma.(gogenconf.Map)
	if !ok || len(mapObj.Entries) != 3 {
		t.Fatalf("map_a = %v; want 3 entries", ma)
	}
	m := map[string]string{}
	for _, e := range mapObj.Entries {
		m[e.Key] = e.Value.(gogenconf.Literal).Value
	}
	if m["k1"] != "v1" || m["k2"] != "updated_v2" || m["k3"] != "new_v3" {
		t.Errorf("map_a contents mismatch: %+v", m)
	}

	// Object [=]
	subA_f1 := effective.PathValue("data", "sub_a", "f1")
	if subA_f1 != nil {
		t.Errorf("sub_a.f1 should have been deleted by [=], got %v", subA_f1)
	}
	subA_f2 := effective.PathValue("data", "sub_a", "f2")
	if lit, ok := subA_f2.(gogenconf.Literal); !ok || lit.Value != "fresh" {
		t.Errorf("sub_a.f2 = %v; want fresh", subA_f2)
	}
}

func TestComposeGo2GentooShaped(t *testing.T) {
	baseText := `config for go2gentoo version 1

repository {
    name gentoo
    overlay_path "./var/overlay"
}

project lxa {
    module_path "github.com/arran4/lxa"
    category app-misc
    package_name lxa
    version "v1.0.0"
    arch [ "amd64", "arm64" ]
    checksum_file "dist/lxa.sha256"

    build {
        target ./cmd/lxa
        cgo "false"
    }
}

project runner {
    module_path "github.com/arran4/runner"
    category sys-apps
    package_name runner
    version "v0.5.0"
    arch [ "amd64" ]

    build {
        target ./cmd/runner
    }
}
`

	overrideText := `config for go2gentoo version 1

[~] repository {
    overlay_path "/etc/portage/overlay"
}

[~] project lxa {
    [+] arch [ "riscv64" ]
    [~] build {
        target ./cmd/lxa-v2
    }
}

[-] project runner {
}

[+] project worker {
    module_path "github.com/arran4/worker"
    category app-misc
    package_name worker
    version "v2.1.0"
    arch [ "amd64" ]

    build {
        target ./cmd/worker
    }
}
`

	baseDoc, err := gogenconf.ParseNamed(strings.NewReader(baseText), "00-base.conf")
	if err != nil {
		t.Fatalf("ParseNamed baseDoc failed: %v", err)
	}
	overrideDoc, err := gogenconf.ParseNamed(strings.NewReader(overrideText), "99-user.conf")
	if err != nil {
		t.Fatalf("ParseNamed overrideDoc failed: %v", err)
	}

	effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "base", Document: baseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "go2gentoo",
		MaxVersion:       1,
	})
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// 1. repository.overlay_path mutated, repository.name preserved
	overlayPath := effective.PathValue("repository", "overlay_path")
	if lit, ok := overlayPath.(gogenconf.Literal); !ok || lit.Value != "/etc/portage/overlay" {
		t.Errorf("repository.overlay_path = %v, want /etc/portage/overlay", overlayPath)
	}
	repoName := effective.PathValue("repository", "name")
	if lit, ok := repoName.(gogenconf.Literal); !ok || lit.Value != "gentoo" {
		t.Errorf("repository.name = %v, want gentoo", repoName)
	}

	// 2. project lxa: arch list appended with riscv64
	archVal := effective.PathValue("project", "lxa", "arch")
	archList, ok := archVal.(gogenconf.List)
	if !ok || len(archList.Items) != 3 {
		t.Fatalf("project lxa arch = %v, want 3 items", archVal)
	}
	if archList.Items[2].(gogenconf.Literal).Value != "riscv64" {
		t.Errorf("project lxa arch[2] = %v, want riscv64", archList.Items[2])
	}

	// 3. project lxa: build.target mutated to ./cmd/lxa-v2, build.cgo preserved as "false"
	buildTarget := effective.PathValue("project", "lxa", "build", "target")
	if lit, ok := buildTarget.(gogenconf.Literal); !ok || lit.Value != "./cmd/lxa-v2" {
		t.Errorf("project lxa build.target = %v, want ./cmd/lxa-v2", buildTarget)
	}
	buildCgo := effective.PathValue("project", "lxa", "build", "cgo")
	if lit, ok := buildCgo.(gogenconf.Literal); !ok || lit.Value != "false" {
		t.Errorf("project lxa build.cgo = %v, want false", buildCgo)
	}

	// 4. project runner deleted via [-]
	if runnerSec := effective.SectionPath("project", "runner"); runnerSec != nil {
		t.Errorf("project runner should have been deleted by [-], but found: %v", runnerSec)
	}

	// 5. project worker added via [+]
	workerSec := effective.SectionPath("project", "worker")
	if workerSec == nil {
		t.Fatalf("project worker should have been added by [+], but not found")
	}
	workerPkg := effective.PathValue("project", "worker", "package_name")
	if lit, ok := workerPkg.(gogenconf.Literal); !ok || lit.Value != "worker" {
		t.Errorf("project worker package_name = %v, want worker", workerPkg)
	}

	// 6. Strict project identity validation: separate CGo mapping document
	cgoBaseText := `config for cgo-mappings version 1

mapping lxa {
    cgo_enabled "true"
    cflags [ "-O2", "-g" ]
    ldflags [ "-s", "-w" ]
}
`
	cgoOverrideText := `config for cgo-mappings version 1

[~] mapping lxa {
    [+] cflags [ "-march=native" ]
}
`
	cgoBaseDoc, err := gogenconf.ParseNamed(strings.NewReader(cgoBaseText), "cgo-base.conf")
	if err != nil {
		t.Fatal(err)
	}
	cgoOverrideDoc, err := gogenconf.ParseNamed(strings.NewReader(cgoOverrideText), "cgo-override.conf")
	if err != nil {
		t.Fatal(err)
	}

	// Incompatible identity rejected when mixed with go2gentoo
	_, err = gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "go2gentoo-base", Document: baseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "cgo-override", Document: cgoOverrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "go2gentoo",
		MaxVersion:       1,
	})
	if err == nil {
		t.Fatal("expected Compose to fail on mismatched identity between go2gentoo and cgo-mappings, got nil")
	}

	// Composing cgo-mappings independently succeeds
	cgoEffective, err := gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "cgo-base", Document: cgoBaseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "cgo-override", Document: cgoOverrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "cgo-mappings",
		MaxVersion:       1,
	})
	if err != nil {
		t.Fatalf("Compose cgo-mappings failed: %v", err)
	}
	cflagsVal := cgoEffective.PathValue("mapping", "lxa", "cflags")
	cflagsList, ok := cflagsVal.(gogenconf.List)
	if !ok || len(cflagsList.Items) != 3 {
		t.Fatalf("mapping lxa cflags = %v, want 3 items", cflagsVal)
	}
	if cflagsList.Items[2].(gogenconf.Literal).Value != "-march=native" {
		t.Errorf("mapping lxa cflags[2] = %v, want -march=native", cflagsList.Items[2])
	}
}

func TestComposeCanonicalDeclarationAttachedSyntax(t *testing.T) {
	baseText := `config for go2gentoo version 1

repository {
    name gentoo
    overlay_path "./var/overlay"
}

project lxa {
    module_path "github.com/arran4/lxa"
    arch [ "amd64", "arm64" ]

    build {
        target ./cmd/lxa
        cgo "false"
    }
}

project runner {
    module_path "github.com/arran4/runner"
}
`

	// Canonical declaration-attached syntax per Issue #25
	canonicalOverrideText := `config for go2gentoo version 1

repository[~] {
    overlay_path[=] "/etc/portage/overlay"
}

project[~] lxa {
    arch[+] [ "riscv64" ]
    build[~] {
        target[=] ./cmd/lxa-v2
    }
}

project runner[-] {
}

project[+] worker {
    module_path "github.com/arran4/worker"
}
`

	baseDoc, err := gogenconf.ParseNamed(strings.NewReader(baseText), "base.conf")
	if err != nil {
		t.Fatalf("parse base failed: %v", err)
	}
	overrideDoc, err := gogenconf.ParseNamed(strings.NewReader(canonicalOverrideText), "override.conf")
	if err != nil {
		t.Fatalf("parse canonical override failed: %v", err)
	}

	effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
		{Name: "base", Document: baseDoc, Priority: gogenconf.PriorityDefault, Order: 0},
		{Name: "override", Document: overrideDoc, Priority: gogenconf.PriorityUser, Order: 0},
	}, gogenconf.CompositionOptions{
		ExpectedIdentity: "go2gentoo",
		MaxVersion:       1,
	})
	if err != nil {
		t.Fatalf("compose failed: %v", err)
	}

	// 1. repository.overlay_path updated to "/etc/portage/overlay", name preserved
	if lit, ok := effective.PathValue("repository", "overlay_path").(gogenconf.Literal); !ok || lit.Value != "/etc/portage/overlay" {
		t.Errorf("overlay_path = %v, want /etc/portage/overlay", effective.PathValue("repository", "overlay_path"))
	}
	if lit, ok := effective.PathValue("repository", "name").(gogenconf.Literal); !ok || lit.Value != "gentoo" {
		t.Errorf("name = %v, want gentoo", effective.PathValue("repository", "name"))
	}

	// 2. project lxa arch appended with riscv64
	archVal := effective.PathValue("project", "lxa", "arch")
	archList, ok := archVal.(gogenconf.List)
	if !ok || len(archList.Items) != 3 {
		t.Fatalf("arch = %v, want 3 items", archVal)
	}
	if archList.Items[2].(gogenconf.Literal).Value != "riscv64" {
		t.Errorf("arch[2] = %v, want riscv64", archList.Items[2])
	}

	// 3. project lxa build target updated, cgo preserved
	if lit, ok := effective.PathValue("project", "lxa", "build", "target").(gogenconf.Literal); !ok || lit.Value != "./cmd/lxa-v2" {
		t.Errorf("build.target = %v, want ./cmd/lxa-v2", effective.PathValue("project", "lxa", "build", "target"))
	}
	if lit, ok := effective.PathValue("project", "lxa", "build", "cgo").(gogenconf.Literal); !ok || lit.Value != "false" {
		t.Errorf("build.cgo = %v, want false", effective.PathValue("project", "lxa", "build", "cgo"))
	}

	// 4. project runner deleted via [-]
	if sec := effective.SectionPath("project", "runner"); sec != nil {
		t.Errorf("project runner should be deleted, got %v", sec)
	}

	// 5. project worker added via [+]
	if sec := effective.SectionPath("project", "worker"); sec == nil {
		t.Errorf("project worker should be added, but not found")
	}
}

func TestComposeCompactNamedMutationTargetsOnlyAddressedDeclaration(t *testing.T) {
	base, err := gogenconf.ParseNamed(strings.NewReader(`config for go2gentoo version 1

project lxa {
	module_path github.com/arran4/lxa
	build {
		target ./cmd/lxa
	}
}

project runner {
	module_path github.com/arran4/runner
}
`), "base.conf")
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	t.Run("delete", func(t *testing.T) {
		override, err := gogenconf.ParseNamed(strings.NewReader(`config for go2gentoo version 1

project[-] lxa {}
`), "delete.conf")
		if err != nil {
			t.Fatalf("parse delete override: %v", err)
		}
		effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
			{Name: "base", Document: base, Priority: gogenconf.PriorityDefault},
			{Name: "delete", Document: override, Priority: gogenconf.PriorityUser},
		}, gogenconf.CompositionOptions{ExpectedIdentity: "go2gentoo", MaxVersion: 1})
		if err != nil {
			t.Fatalf("compose delete: %v", err)
		}
		if effective.SectionPath("project", "lxa") != nil {
			t.Fatal("project[-] lxa retained lxa")
		}
		if effective.SectionPath("project", "runner") == nil {
			t.Fatal("project[-] lxa removed unrelated runner")
		}
	})

	t.Run("replace", func(t *testing.T) {
		override, err := gogenconf.ParseNamed(strings.NewReader(`config for go2gentoo version 1

project[=] lxa {
	module_path github.com/arran4/lxa-v2
}
`), "replace.conf")
		if err != nil {
			t.Fatalf("parse replace override: %v", err)
		}
		effective, err := gogenconf.Compose([]gogenconf.SourceLayer{
			{Name: "base", Document: base, Priority: gogenconf.PriorityDefault},
			{Name: "replace", Document: override, Priority: gogenconf.PriorityUser},
		}, gogenconf.CompositionOptions{ExpectedIdentity: "go2gentoo", MaxVersion: 1})
		if err != nil {
			t.Fatalf("compose replace: %v", err)
		}
		if got, ok := effective.PathValue("project", "lxa", "module_path").(gogenconf.Literal); !ok || got.Value != "github.com/arran4/lxa-v2" {
			t.Fatalf("replaced lxa.module_path = %#v", effective.PathValue("project", "lxa", "module_path"))
		}
		if effective.SectionPath("project", "lxa", "build") != nil {
			t.Fatal("project[=] lxa retained lxa's old build section")
		}
		if effective.SectionPath("project", "runner") == nil {
			t.Fatal("project[=] lxa replaced unrelated runner")
		}
	})
}
