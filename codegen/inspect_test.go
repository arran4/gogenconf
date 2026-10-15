package codegen_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arran4/gogenconf/codegen"
)

const inspectTestSchema = `config for downloadsd-schema version 1

section service
    ## Service daemon settings
    host string
    port string
    api_key string
end

section storage
    path string
    temp_dir string
end
`

func TestInspectSchema(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.conf")
	if err := os.WriteFile(schemaPath, []byte(inspectTestSchema), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := codegen.InspectSchemaFile(schemaPath)
	if err != nil {
		t.Fatalf("InspectSchemaFile failed: %v", err)
	}

	if info.Identity != "downloadsd-schema" {
		t.Errorf("expected identity downloadsd-schema, got %q", info.Identity)
	}
	if info.Version != 1 {
		t.Errorf("expected version 1, got %d", info.Version)
	}
	if len(info.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(info.Sections))
	}

	sec0 := info.Sections[0]
	if sec0.Name != "service" || sec0.GoName != "Service" {
		t.Errorf("unexpected section 0: %v", sec0)
	}
	if len(sec0.Fields) != 3 {
		t.Errorf("expected 3 fields in service, got %d", len(sec0.Fields))
	}

	text := info.FormatText()
	if !strings.Contains(text, "Section: service") || !strings.Contains(text, "host -> Host: string") {
		t.Errorf("unexpected text outline:\n%s", text)
	}

	data, err := info.JSON()
	if err != nil {
		t.Fatalf("JSON failed: %v", err)
	}

	var parsed struct {
		Identity string `json:"identity"`
		Sections []struct {
			Name string `json:"name"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed unmarshaling json: %v", err)
	}
	if parsed.Identity != "downloadsd-schema" || len(parsed.Sections) != 2 {
		t.Errorf("unexpected parsed json: %+v", parsed)
	}
}

func TestLoadAndValidateSchemaErrors(t *testing.T) {
	tmpDir := t.TempDir()
	badSchema := `config for downloadsd-schema version 1
section service
    host unsupported_type_xyz
end
`
	schemaPath := filepath.Join(tmpDir, "bad.conf")
	if err := os.WriteFile(schemaPath, []byte(badSchema), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := codegen.LoadAndValidateSchema(schemaPath)
	if err == nil {
		t.Fatal("expected LoadAndValidateSchema to fail on unsupported type, but got nil")
	}
}
