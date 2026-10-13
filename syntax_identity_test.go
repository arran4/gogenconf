package gogenconf_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
)

func TestConfigIdentityAndVersionHeader(t *testing.T) {
	input := `config for go2gentoo version 1
endpoint https://example.test
`
	doc, err := gogenconf.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Identity != "go2gentoo" {
		t.Errorf("expected identity 'go2gentoo', got %q", doc.Identity)
	}
	if doc.Version != 1 {
		t.Errorf("expected version 1, got %d", doc.Version)
	}
	if !doc.HasVersion {
		t.Errorf("expected HasVersion true")
	}
	if doc.HeaderSpan.Line != 1 || doc.HeaderSpan.Column != 1 {
		t.Errorf("expected header span 1:1, got %v", doc.HeaderSpan)
	}

	formatted := gogenconf.Format(doc)
	if !strings.HasPrefix(formatted, "config for go2gentoo version 1\n") {
		t.Errorf("unexpected formatted header:\n%s", formatted)
	}
}

func TestCheckCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedIdent string
		maxVersion    int
		wantErr       string
	}{
		{
			name:          "matching identity and valid version",
			input:         "config for go2gentoo version 1\n",
			expectedIdent: "go2gentoo",
			maxVersion:    2,
			wantErr:       "",
		},
		{
			name:          "matching identity and max version boundary",
			input:         "config for go2gentoo version 2\n",
			expectedIdent: "go2gentoo",
			maxVersion:    2,
			wantErr:       "",
		},
		{
			name:          "mismatched identity",
			input:         "config for cgo-mappings version 1\n",
			expectedIdent: "go2gentoo",
			maxVersion:    2,
			wantErr:       "expected config for go2gentoo version <= 2; got config for cgo-mappings version 1",
		},
		{
			name:          "version exceeds maxVersion",
			input:         "config for go2gentoo version 3\n",
			expectedIdent: "go2gentoo",
			maxVersion:    2,
			wantErr:       "expected config for go2gentoo version <= 2; got version 3",
		},
		{
			name:          "unversioned document",
			input:         "timeout 30s\n",
			expectedIdent: "go2gentoo",
			maxVersion:    2,
			wantErr:       "expected config for go2gentoo version <= 2; got unversioned document",
		},
		{
			name:          "legacy config_version document",
			input:         "config_version 1\n",
			expectedIdent: "go2gentoo",
			maxVersion:    2,
			wantErr:       "expected config for go2gentoo version <= 2; got legacy config_version 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := gogenconf.Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			compatErr := doc.CheckCompatibility(tt.expectedIdent, tt.maxVersion)
			if tt.wantErr == "" {
				if compatErr != nil {
					t.Fatalf("expected compatibility success, got: %v", compatErr)
				}
				return
			}
			if compatErr == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if !strings.Contains(compatErr.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain expected substring %q", compatErr.Error(), tt.wantErr)
			}

			// Verify it is a *ConfigError with StageValidate
			var cfgErr *gogenconf.ConfigError
			if !errors.As(compatErr, &cfgErr) {
				t.Errorf("expected *ConfigError, got %T", compatErr)
			} else if cfgErr.Stage != gogenconf.StageValidate {
				t.Errorf("expected StageValidate, got %s", cfgErr.Stage)
			}
		})
	}
}

func TestRootScalarConfiguration(t *testing.T) {
	input := `config for downloadsd version 1

# Service level limits
timeout 30s
retries 3
endpoint "https://download.example.test/api"

section storage
    path /var/lib/downloads
end
`
	doc, err := gogenconf.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify root entries
	entry, idx, ok := doc.Entry("timeout")
	if !ok || idx < 0 {
		t.Fatalf("root entry 'timeout' not found")
	}
	if entry.Key != "timeout" {
		t.Errorf("expected key 'timeout', got %q", entry.Key)
	}
	lit, ok := entry.Value.(gogenconf.Literal)
	if !ok || lit.Value != "30s" {
		t.Errorf("expected literal '30s', got %v", entry.Value)
	}

	// Value("", "timeout")
	val := doc.Value("", "timeout")
	if val == nil {
		t.Fatalf("Value('', 'timeout') returned nil")
	}

	// Root Set and Delete
	doc.Set("retries", gogenconf.Literal{Value: "5"})
	newRetries, _, ok := doc.Entry("retries")
	if !ok || newRetries.Value.(gogenconf.Literal).Value != "5" {
		t.Errorf("Set failed on root entry: %v", newRetries)
	}

	if !doc.Delete("endpoint") {
		t.Errorf("Delete returned false for 'endpoint'")
	}
	if _, _, ok := doc.Entry("endpoint"); ok {
		t.Errorf("'endpoint' still exists after Delete")
	}

	// Section entry should still work
	storageSec := doc.Section("storage")
	if storageSec == nil {
		t.Fatalf("section 'storage' not found")
	}
	pathVal := doc.Value("storage", "path")
	if pathVal == nil || pathVal.(gogenconf.Literal).Value != "/var/lib/downloads" {
		t.Errorf("section storage.path mismatch: %v", pathVal)
	}

	// Formatting
	formatted := gogenconf.Format(doc)
	if !strings.Contains(formatted, "timeout 30s") {
		t.Errorf("formatted document missing root entry 'timeout 30s':\n%s", formatted)
	}
	if !strings.Contains(formatted, "retries 5") {
		t.Errorf("formatted document missing updated root entry 'retries 5':\n%s", formatted)
	}
	if strings.Contains(formatted, "endpoint") {
		t.Errorf("formatted document contains deleted 'endpoint':\n%s", formatted)
	}
}

func TestDuplicateRootFieldsPreservedInAST(t *testing.T) {
	input := `config for testapp version 1
timeout 30s
timeout 60s
`
	doc, err := gogenconf.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error parsing duplicate root fields: %v", err)
	}
	entries := doc.Entries("timeout")
	if len(entries) != 2 {
		t.Fatalf("expected 2 duplicate entries preserved in AST, got %d", len(entries))
	}
	if lit, ok := entries[0].Value.(gogenconf.Literal); !ok || lit.Value != "30s" {
		t.Errorf("entry 0 mismatch: %v", entries[0].Value)
	}
	if lit, ok := entries[1].Value.(gogenconf.Literal); !ok || lit.Value != "60s" {
		t.Errorf("entry 1 mismatch: %v", entries[1].Value)
	}
}

func TestHeaderValidationErrors(t *testing.T) {
	malformed := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "incomplete header",
			input:   "config for myapp\n",
			wantErr: "invalid config header",
		},
		{
			name:    "missing version number",
			input:   "config for myapp version\n",
			wantErr: "invalid config header",
		},
		{
			name:    "negative version number",
			input:   "config for myapp version -1\n",
			wantErr: "invalid config version in header",
		},
		{
			name:    "non-numeric version",
			input:   "config for myapp version alpha\n",
			wantErr: "invalid config version in header",
		},
		{
			name:    "invalid characters in identity",
			input:   "config for my$app version 1\n",
			wantErr: "invalid configuration identity",
		},
		{
			name:    "duplicate header",
			input:   "config for app1 version 1\nconfig for app2 version 2\n",
			wantErr: "config header must occur once at root",
		},
		{
			name:    "header inside section",
			input:   "section foo\nconfig for app version 1\nend\n",
			wantErr: "config header must occur once at root",
		},
	}

	for _, tt := range malformed {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gogenconf.Parse(strings.NewReader(tt.input))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
