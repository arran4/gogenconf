package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arran4/gogenconf/internal/cli"
)

const docIntegrationSchema = `config for downloadsd-schema version 1

section service
    ## Background download manager service
    host string
    port string
    ## Secret authentication token
    token string
end

section network
    ## Network proxy settings
    proxy string
end
`

func TestDocCLICommandEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.conf")
	if err := os.WriteFile(schemaPath, []byte(docIntegrationSchema), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "docs")

	// 1. Run cli.Doc programmatically
	err := cli.Doc(schemaPath, "", outDir, true, true, true, false, "downloadsd")
	if err != nil {
		t.Fatalf("cli.Doc failed: %v", err)
	}

	expectedFiles := []string{
		"downloadsd.md",
		"downloadsd.5",
		"sample.conf",
	}

	for _, name := range expectedFiles {
		p := filepath.Join(outDir, name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected generated doc file %s to exist: %v", name, err)
		}
	}

	// 2. Test check mode on fresh files
	err = cli.Doc(schemaPath, "", outDir, true, true, true, true, "downloadsd")
	if err != nil {
		t.Fatalf("cli.Doc --check failed on fresh files: %v", err)
	}

	// 3. Test check mode detects tampered file
	mdFile := filepath.Join(outDir, "downloadsd.md")
	if err := os.WriteFile(mdFile, []byte("# Stale Title"), 0644); err != nil {
		t.Fatal(err)
	}
	err = cli.Doc(schemaPath, "", outDir, true, true, true, true, "downloadsd")
	if err == nil {
		t.Fatal("expected cli.Doc --check to fail on modified file, got nil")
	}

	// 4. Test execution via binary command
	binDir := t.TempDir()
	rootPath, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/gogenconf", "doc", "--schema", schemaPath, "--out", binDir, "--markdown", "--man", "--sample")
	cmd.Dir = rootPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cmd/gogenconf doc failed: %v\nOutput: %s", err, string(out))
	}

	for _, name := range expectedFiles {
		p := filepath.Join(binDir, name)
		content, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("binary doc execution missing file %s: %v", name, err)
			continue
		}
		if name == "downloadsd.md" && !strings.Contains(string(content), "# downloadsd Configuration Reference") {
			t.Errorf("unexpected markdown content in %s", name)
		}
		if name == "downloadsd.5" && !strings.Contains(string(content), ".TH DOWNLOADSD.5 5") {
			t.Errorf("unexpected man content in %s", name)
		}
	}
}
