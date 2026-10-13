package gogenconf_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
)

func TestTrustedMaxIntConstraint(t *testing.T) {
	sysDoc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    max_parallel 5
end
`), "10-system.conf")
	if err != nil {
		t.Fatal(err)
	}

	userDoc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    max_parallel 100
end
`), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}

	layers := []gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDoc, Priority: gogenconf.PriorityUser, Trusted: false},
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.RequireMaxInt("service.max_parallel", 16, "system-policy")

	// 1. User requests 100 which exceeds max 16 -> must fail
	_, err = gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err == nil {
		t.Fatal("expected policy violation error for max_parallel, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds trusted maximum limit 16") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), "enforced by system-policy") {
		t.Fatalf("error missing enforcing authority: %v", err)
	}

	// 2. User requests 8 which is within max 16 -> must succeed
	userDocValid, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    max_parallel 8
end
`), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}
	layers[1].Document = userDocValid

	effective, err := gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err != nil {
		t.Fatalf("expected valid config to succeed, got %v", err)
	}
	if lit, ok := effective.PathValue("service", "max_parallel").(gogenconf.Literal); !ok || lit.Value != "8" {
		t.Errorf("effective max_parallel = %v; want 8", effective.PathValue("service", "max_parallel"))
	}
}

func TestTrustedAllowlistConstraint(t *testing.T) {
	sysDoc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    allowed_roots [ "/var/downloads", "/mnt/shared" ]
end
`), "10-system.conf")
	if err != nil {
		t.Fatal(err)
	}

	userDocIllegal, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    target_dir "/etc/shadow"
end
`), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}

	layers := []gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDocIllegal, Priority: gogenconf.PriorityUser, Trusted: false},
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.RequireAllowedStrings("service.target_dir", []string{"/var/downloads", "/mnt/shared"}, "machine-security")

	_, err = gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err == nil {
		t.Fatal("expected allowlist violation error, got nil")
	}
	if !strings.Contains(err.Error(), "not in trusted allowlist") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUntrustedLayerSystemOnlyFieldRejected(t *testing.T) {
	sysDoc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    name "downloadsd-daemon"
end
`), "10-system.conf")
	if err != nil {
		t.Fatal(err)
	}

	// Untrusted user configuration attempts to declare a system-only token file
	userDoc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section service
    privileged_token_file "/tmp/fake_root.token"
end
`), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}

	layers := []gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDoc, Priority: gogenconf.PriorityUser, Trusted: false},
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.MarkSystemOnly("service.privileged_token_file", "root-daemon")

	_, err = gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err == nil {
		t.Fatal("expected system-only field rejection for untrusted layer, got nil")
	}
	if !strings.Contains(err.Error(), `untrusted layer "user" cannot configure system-only field "service.privileged_token_file"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSensitiveConstraintValueRedaction(t *testing.T) {
	userDoc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section auth
    api_key "super_secret_forbidden_token"
end
`), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}

	layers := []gogenconf.SourceLayer{
		{Name: "user", Document: userDoc, Priority: gogenconf.PriorityUser, Trusted: false},
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.MarkSensitive("auth.api_key")
	policy.RequireAllowedStrings("auth.api_key", []string{"safe_token_1", "safe_token_2"}, "auth-policy")

	_, err = gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err == nil {
		t.Fatal("expected allowlist failure, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "super_secret_forbidden_token") {
		t.Fatalf("sensitive credential was leaked in error message: %s", errMsg)
	}
	if !strings.Contains(errMsg, "[REDACTED]") {
		t.Fatalf("error message missing [REDACTED] marker: %s", errMsg)
	}
}

func TestCustomApplicationValidator(t *testing.T) {
	doc, err := gogenconf.ParseNamed(strings.NewReader(`config for downloadsd version 1
section scheduler
    queue_mode "fifo"
end
`), "app.conf")
	if err != nil {
		t.Fatal(err)
	}

	layers := []gogenconf.SourceLayer{
		{Name: "app", Document: doc, Priority: gogenconf.PriorityUser, Trusted: false},
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.AddValidator(func(effective *gogenconf.Document) error {
		mode := effective.PathValue("scheduler", "queue_mode")
		if lit, ok := mode.(gogenconf.Literal); ok && lit.Value == "fifo" {
			return nil
		}
		return fmt.Errorf("unsupported queue mode")
	})

	effective, err := gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err != nil {
		t.Fatalf("custom validator failed unexpectedly: %v", err)
	}
	if effective == nil {
		t.Fatal("expected non-nil effective document")
	}
}

func TestFailClosedUnsupportedExpressionShape(t *testing.T) {
	doc := &gogenconf.Document{
		Identity:   "downloadsd",
		Version:    1,
		HasVersion: true,
	}
	sec := &gogenconf.Section{Name: "network"}
	sec.Items = append(sec.Items, gogenconf.Entry{
		Key:   "allowed_host",
		Value: gogenconf.Call{Name: "unresolved_call", Args: []gogenconf.Expr{gogenconf.Literal{Value: "foo"}}},
	})
	doc.AddSection(sec)

	layers := []gogenconf.SourceLayer{
		{Name: "app.conf", Document: doc, Priority: gogenconf.PriorityUser, Order: 0, Trusted: false},
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.RequireAllowedStrings("network.allowed_host", []string{"example.com"}, "net-policy")

	_, err := gogenconf.ComposeWithPolicy(layers, gogenconf.CompositionOptions{
		ExpectedIdentity: "downloadsd",
		MaxVersion:       1,
	}, policy)
	if err == nil {
		t.Fatal("expected policy rejection on unsupported expression shape, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported expression shape") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestPolicyConstraintsWithNativeMutationEngine(t *testing.T) {
	sysText := `config for app version 1

service storage {
    quota 200
    driver "local"
    root_mount "/var/data"
}
`
	sysDoc, err := gogenconf.ParseNamed(strings.NewReader(sysText), "10-system.conf")
	if err != nil {
		t.Fatal(err)
	}

	policy := gogenconf.NewPolicyEnvelope()
	policy.RequireMaxInt("service.storage.quota", 500, "storage-guard")
	policy.RequireAllowedStrings("service.storage.driver", []string{"local", "s3"}, "storage-guard")
	policy.RequireDeniedStrings("service.storage.api_key", []string{"forbidden-secret-token"}, "secret-policy")
	policy.MarkSensitive("service.storage.api_key")
	policy.MarkSystemOnly("service.storage.root_mount", "storage-admin")

	// Case A: User attempts [=] with value exceeding max limit (quota 1000)
	userTextExcess := `config for app version 1

service storage {
    [=] quota 1000
}
`
	userDocExcess, err := gogenconf.ParseNamed(strings.NewReader(userTextExcess), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}
	_, err = gogenconf.ComposeWithPolicy([]gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDocExcess, Priority: gogenconf.PriorityUser, Trusted: false},
	}, gogenconf.CompositionOptions{ExpectedIdentity: "app", MaxVersion: 1}, policy)
	if err == nil || !strings.Contains(err.Error(), "exceeds trusted maximum limit 500") {
		t.Fatalf("expected max limit rejection, got %v", err)
	}

	// Case B: User attempts [~] with disallowed driver using compact path
	userTextDisallowed := `config for app version 1

service storage {
    [~] driver "ftp"
}
`
	userDocDisallowed, err := gogenconf.ParseNamed(strings.NewReader(userTextDisallowed), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}
	_, err = gogenconf.ComposeWithPolicy([]gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDocDisallowed, Priority: gogenconf.PriorityUser, Trusted: false},
	}, gogenconf.CompositionOptions{ExpectedIdentity: "app", MaxVersion: 1}, policy)
	if err == nil || !strings.Contains(err.Error(), "not in trusted allowlist") {
		t.Fatalf("expected allowlist rejection, got %v", err)
	}

	// Case C: User attempts system-only override via compact path
	userTextSysOnly := `config for app version 1

service storage {
    [=] root_mount "/mnt/user"
}
`
	userDocSysOnly, err := gogenconf.ParseNamed(strings.NewReader(userTextSysOnly), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}
	_, err = gogenconf.ComposeWithPolicy([]gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDocSysOnly, Priority: gogenconf.PriorityUser, Trusted: false},
	}, gogenconf.CompositionOptions{ExpectedIdentity: "app", MaxVersion: 1}, policy)
	if err == nil || !strings.Contains(err.Error(), "cannot configure system-only field") {
		t.Fatalf("expected system-only rejection, got %v", err)
	}

	// Case D: Zero secret leakage on denied sensitive string
	userTextSecret := `config for app version 1

service storage {
    [+] api_key "forbidden-secret-token"
}
`
	userDocSecret, err := gogenconf.ParseNamed(strings.NewReader(userTextSecret), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}
	_, err = gogenconf.ComposeWithPolicy([]gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDocSecret, Priority: gogenconf.PriorityUser, Trusted: false},
	}, gogenconf.CompositionOptions{ExpectedIdentity: "app", MaxVersion: 1}, policy)
	if err == nil {
		t.Fatal("expected secret rejection, got nil")
	}
	if strings.Contains(err.Error(), "forbidden-secret-token") {
		t.Fatalf("sensitive token leaked in error message: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in error, got: %s", err.Error())
	}

	// Case E: Valid mutation within bounds succeeds
	userTextValid := `config for app version 1

service storage {
    [=] quota 400
    [=] driver "s3"
}
`
	userDocValid, err := gogenconf.ParseNamed(strings.NewReader(userTextValid), "99-user.conf")
	if err != nil {
		t.Fatal(err)
	}
	effective, err := gogenconf.ComposeWithPolicy([]gogenconf.SourceLayer{
		{Name: "system", Document: sysDoc, Priority: gogenconf.PrioritySystem, Trusted: true},
		{Name: "user", Document: userDocValid, Priority: gogenconf.PriorityUser, Trusted: false},
	}, gogenconf.CompositionOptions{ExpectedIdentity: "app", MaxVersion: 1}, policy)
	if err != nil {
		t.Fatalf("expected valid mutation to pass policy, got: %v", err)
	}
	qVal := effective.PathValue("service", "storage", "quota")
	if lit, ok := qVal.(gogenconf.Literal); !ok || lit.Value != "400" {
		t.Errorf("service.storage.quota = %v; want 400", qVal)
	}
}
