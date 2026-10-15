package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const inspectIntegrationSchema = `config for downloadsd-schema version 1

section service
    ## Background download service daemon
    host string
    port string
end

section database
    ## Database storage
    dsn string
end
`

func TestSchemaCLICommands(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.conf")
	if err := os.WriteFile(schemaPath, []byte(inspectIntegrationSchema), 0644); err != nil {
		t.Fatal(err)
	}

	rootPath, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Test gogenconf schema list
	listCmd := exec.Command("go", "run", "./cmd/gogenconf", "schema", "list", "--schema", schemaPath)
	listCmd.Dir = rootPath
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema list failed: %v\nOutput: %s", err, string(listOut))
	}
	if !strings.Contains(string(listOut), "Schema: downloadsd-schema") || !strings.Contains(string(listOut), "Section: service") {
		t.Errorf("unexpected schema list output: %s", string(listOut))
	}

	// 2. Test gogenconf list alias with --json
	jsonCmd := exec.Command("go", "run", "./cmd/gogenconf", "list", "--json", schemaPath)
	jsonCmd.Dir = rootPath
	jsonOut, err := jsonCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list --json failed: %v\nOutput: %s", err, string(jsonOut))
	}

	var parsed struct {
		Identity string `json:"identity"`
		Sections []struct {
			Name string `json:"name"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(jsonOut, &parsed); err != nil {
		t.Fatalf("failed unmarshaling json output: %v\nOutput: %s", err, string(jsonOut))
	}
	if parsed.Identity != "downloadsd-schema" || len(parsed.Sections) != 2 {
		t.Errorf("unexpected json parsed data: %+v", parsed)
	}

	// 3. Test gogenconf schema validate success
	valCmd := exec.Command("go", "run", "./cmd/gogenconf", "schema", "validate", "--schema", schemaPath)
	valCmd.Dir = rootPath
	valOut, err := valCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema validate failed on valid schema: %v\nOutput: %s", err, string(valOut))
	}

	// 4. Test gogenconf schema validate on invalid schema
	badPath := filepath.Join(tmpDir, "bad.conf")
	badSchema := `config for downloadsd-schema version 1
section service
    host invalid_type_mapping
end
`
	if err := os.WriteFile(badPath, []byte(badSchema), 0644); err != nil {
		t.Fatal(err)
	}
	badCmd := exec.Command("go", "run", "./cmd/gogenconf", "schema", "validate", badPath)
	badCmd.Dir = rootPath
	badOut, err := badCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected schema validate to fail on invalid schema, got zero exit. Output: %s", string(badOut))
	}

	// 5. Test gogenconf schema plan alias
	planCmd := exec.Command("go", "run", "./cmd/gogenconf", "schema", "plan", "--schema", schemaPath)
	planCmd.Dir = rootPath
	planOut, err := planCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema plan failed: %v\nOutput: %s", err, string(planOut))
	}
	if !strings.Contains(string(planOut), "Generation Plan:") {
		t.Errorf("unexpected plan output: %s", string(planOut))
	}

	// 6. Verify no files were created or modified in schema directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 { // only schema.conf and bad.conf
		t.Errorf("unexpected files left in temp dir: %d", len(entries))
	}
}
