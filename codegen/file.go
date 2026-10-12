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

// WriteToDir writes all generated files into dir with safe staged transactions.
// It verifies destination safety against user-authored extension files, stages all writes,
// and ensures all-or-nothing rollback on any write or rename failure.
func (fs FileSet) WriteToDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// 1. Safety verification: ensure no user-authored extension files or symlinks would be overwritten without permission
	for _, f := range fs {
		dest := filepath.Join(dir, f.Name)
		if info, err := os.Lstat(dest); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to write to unsafe symlink destination %q", dest)
			}
			if !info.IsDir() && !isGeneratedByGogenconf(dest) {
				return fmt.Errorf("refusing to overwrite user-authored or conflicting file %q (missing gogenconf generated header)", dest)
			}
		}
	}

	// 2. Stage all files into temporary files first
	type stagedFile struct {
		dest   string
		tmp    string
		backup string
		exists bool
	}

	var staged []stagedFile
	cleanupTmp := func() {
		for _, s := range staged {
			_ = os.Remove(s.tmp)
		}
	}

	pid := os.Getpid()
	for _, f := range fs {
		dest := filepath.Join(dir, f.Name)
		tmp := fmt.Sprintf("%s.tmp.%d.%s", dest, pid, filepath.Base(f.Name))
		if err := os.WriteFile(tmp, f.Content, 0644); err != nil {
			cleanupTmp()
			return fmt.Errorf("stage file %s: %w", tmp, err)
		}
		info, err := os.Stat(dest)
		exists := (err == nil && !info.IsDir())
		staged = append(staged, stagedFile{
			dest:   dest,
			tmp:    tmp,
			backup: fmt.Sprintf("%s.bak.%d.%s", dest, pid, filepath.Base(f.Name)),
			exists: exists,
		})
	}

	// 3. Atomically apply changes with rollback on failure
	var applied []stagedFile
	rollback := func() {
		for _, s := range applied {
			if s.exists {
				_ = os.Rename(s.backup, s.dest)
			} else {
				_ = os.Remove(s.dest)
			}
			_ = os.Remove(s.backup)
		}
		cleanupTmp()
	}

	for _, s := range staged {
		if s.exists {
			if err := os.Rename(s.dest, s.backup); err != nil {
				rollback()
				return fmt.Errorf("backup existing file %s: %w", s.dest, err)
			}
		}
		if err := os.Rename(s.tmp, s.dest); err != nil {
			if s.exists {
				_ = os.Rename(s.backup, s.dest)
			}
			rollback()
			return fmt.Errorf("apply generated file %s: %w", s.dest, err)
		}
		applied = append(applied, s)
	}

	// 4. Success: clean up temporary backup files
	for _, s := range applied {
		if s.exists {
			_ = os.Remove(s.backup)
		}
	}

	return nil
}
