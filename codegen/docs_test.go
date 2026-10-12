package codegen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

const docTestSchema = `config for downloadsd-schema version 1

section service
    ## Download daemon service settings
    host string
    port string
    ## Secret authentication token for API access
    api_token string
end

section storage
    ## Local storage paths
    path string
end
`

func TestGenerateMarkdown(t *testing.T) {
	s, err := m.ParseSchemaNamed(strings.NewReader(docTestSchema), "test.conf")
	if err != nil {
		t.Fatal(err)
	}

	out, err := codegen.GenerateMarkdown(s, codegen.DocOptions{
		AppName: "downloadsd",
	})
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	content := string(out)
	if !strings.Contains(content, "# downloadsd Configuration Reference") {
		t.Errorf("missing title in markdown: %s", content)
	}
	if !strings.Contains(content, "## service") {
		t.Errorf("missing section header: %s", content)
	}
	if !strings.Contains(content, "| `host` | `string` |") {
		t.Errorf("missing field table row: %s", content)
	}
}

func TestGenerateManPage(t *testing.T) {
	s, err := m.ParseSchemaNamed(strings.NewReader(docTestSchema), "test.conf")
	if err != nil {
		t.Fatal(err)
	}

	out, err := codegen.GenerateManPage(s, codegen.ManOptions{
		AppName: "downloadsd",
		Section: 5,
	})
	if err != nil {
		t.Fatalf("GenerateManPage failed: %v", err)
	}

	content := string(out)
	if !strings.Contains(content, ".TH DOWNLOADSD.5 5") {
		t.Errorf("missing .TH header in man page: %s", content)
	}
	if !strings.Contains(content, ".SH SECTIONS") {
		t.Errorf("missing .SH SECTIONS in man page: %s", content)
	}
	if !strings.Contains(content, ".SS service") {
		t.Errorf("missing .SS service in man page: %s", content)
	}
}

func TestPlanDocsAndCheckMode(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.conf")
	if err := os.WriteFile(schemaPath, []byte(docTestSchema), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "docs")

	opts := codegen.DocProjectOptions{
		SchemaFile:   schemaPath,
		OutputDir:    outDir,
		EmitMarkdown: true,
		EmitMan:      true,
		EmitSample:   true,
	}

	plan, err := codegen.PlanDocs(opts)
	if err != nil {
		t.Fatalf("PlanDocs failed: %v", err)
	}

	if len(plan.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(plan.Files))
	}

	if err := plan.Execute(opts); err != nil {
		t.Fatalf("plan.Execute failed: %v", err)
	}

	for _, f := range plan.Files {
		p := filepath.Join(outDir, f.Name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("file %s missing on disk: %v", f.Name, err)
		}
	}

	// Test check mode
	opts.CheckOnly = true
	if err := plan.Execute(opts); err != nil {
		t.Fatalf("check mode failed on valid files: %v", err)
	}

	// Test check mode detects stale file
	mdFile := filepath.Join(outDir, plan.Files[0].Name)
	if err := os.WriteFile(mdFile, []byte("# Stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := plan.Execute(opts); err == nil {
		t.Fatal("expected check mode to fail on stale file, got nil")
	}
}
