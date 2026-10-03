package codegen

import (
	"fmt"
	"os"
	"path/filepath"
)

// GeneratedFile represents a single generated source artifact.
type GeneratedFile struct {
	Name    string // e.g. "config_document_generated.go"
	Content []byte // formatted Go source
}

// FileSet is a collection of generated source files.
type FileSet []GeneratedFile

// File returns the file matching the given name, or nil if not found.
func (fs FileSet) File(name string) *GeneratedFile {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}

// WriteToDir writes all generated files into dir, creating the directory if needed.
// It writes atomically using temporary files to avoid partial state on failure.
func (fs FileSet) WriteToDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	for _, f := range fs {
		dest := filepath.Join(dir, f.Name)
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, f.Content, 0644); err != nil {
			return fmt.Errorf("write temporary %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("rename %s to %s: %w", tmp, dest, err)
		}
	}
	return nil
}
