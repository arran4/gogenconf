package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseTagRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash release workflow runs on Linux")
	}
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	repo := filepath.Join(dir, "repo")
	run(t, dir, nil, "", "git", "init", "--bare", remote)
	run(t, dir, nil, "", "git", "clone", remote, repo)
	run(t, repo, nil, "", "git", "config", "user.email", "test@example.test")
	run(t, repo, nil, "", "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, nil, "", "git", "add", "README")
	run(t, repo, nil, "", "git", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	sha := strings.TrimSpace(run(t, repo, nil, "", "git", "rev-parse", "HEAD"))
	script := filepath.Join(root(t), "scripts/release-tag.sh")
	run(t, repo, nil, "", "bash", script, "v0.0.1-test.1", sha)
	run(t, repo, nil, "", "bash", script, "v0.0.1-test.1", sha) // safe retry
	run(t, repo, nil, "", "git", "-c", "tag.gpgsign=false", "tag", "-a", "v0.0.1-alpha.1", "-m", "annotated", sha)
	run(t, repo, nil, "", "git", "push", "origin", "v0.0.1-alpha.1")
	run(t, repo, nil, "", "bash", script, "v0.0.1-alpha.1", sha) // peeled annotated tag
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dirty := exec.Command("bash", script, "v0.0.2", sha)
	dirty.Dir = repo
	if err := dirty.Run(); err == nil {
		t.Fatal("wrong/dirty checkout accepted")
	}
	run(t, repo, nil, "", "git", "add", "README")
	run(t, repo, nil, "", "git", "-c", "commit.gpgsign=false", "commit", "-m", "changed")
	second := strings.TrimSpace(run(t, repo, nil, "", "git", "rev-parse", "HEAD"))
	c := exec.Command("bash", script, "v0.0.1-test.1", second)
	c.Dir = repo
	if err := c.Run(); err == nil {
		t.Fatal("conflicting tag overwritten")
	}
}
