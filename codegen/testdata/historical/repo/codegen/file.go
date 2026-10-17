package codegen

import (
	"errors"
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

// WriteToDir stages and applies all generated files as one recoverable
// transaction. It never overwrites handwritten files or symlinks.
func (fs FileSet) WriteToDir(dir string) error { return fs.writeTransaction(dir, nil) }

type transactionFile struct {
	dest, stage, backup        string
	existed, backedUp, applied bool
}

func validOutputName(name string) bool {
	return name != "" && !filepath.IsAbs(name) && filepath.Clean(name) == name && filepath.Base(name) == name && name != "."
}

func ensureSafeOutputDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect output directory %q: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("output directory %q must be a real directory, not a symlink", dir)
	}
	return nil
}

func outputMode(path string) (os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return 0644, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, false, fmt.Errorf("refusing to write to unsafe symlink destination %q", path)
	}
	if !info.Mode().IsRegular() {
		return 0, false, fmt.Errorf("refusing unsafe non-regular destination %q", path)
	}
	if !isGeneratedByGogenconf(path) {
		return 0, false, fmt.Errorf("refusing to overwrite user-authored or conflicting file %q (missing gogenconf generated header)", path)
	}
	return info.Mode().Perm(), true, nil
}

// writeTransaction also removes obsolete generated files. Deletions are backed
// up before any staged output is applied, so a failed clean cannot leave a
// partially regenerated directory.
func (fs FileSet) writeTransaction(dir string, obsolete []string) error {
	if err := ensureSafeOutputDir(dir); err != nil {
		return err
	}
	seen := make(map[string]bool, len(fs)+len(obsolete))
	entries := make([]transactionFile, 0, len(fs)+len(obsolete))
	for _, f := range fs {
		if !validOutputName(f.Name) || seen[f.Name] {
			return fmt.Errorf("invalid or duplicate generated output name %q", f.Name)
		}
		seen[f.Name] = true
		dest := filepath.Join(dir, f.Name)
		mode, existed, err := outputMode(dest)
		if err != nil {
			return err
		}
		stage, err := os.CreateTemp(dir, ".gogenconf-stage-"+f.Name+"-*")
		if err != nil {
			return fmt.Errorf("create exclusive stage for %q: %w", dest, err)
		}
		if err = stage.Chmod(mode); err == nil {
			_, err = stage.Write(f.Content)
		}
		if err == nil {
			err = stage.Sync()
		}
		if closeErr := stage.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(stage.Name())
			for _, entry := range entries {
				_ = os.Remove(entry.stage)
			}
			return fmt.Errorf("stage generated output %q: %w", dest, err)
		}
		entries = append(entries, transactionFile{dest: dest, stage: stage.Name(), existed: existed})
	}
	for _, name := range obsolete {
		if !validOutputName(name) || seen[name] {
			return fmt.Errorf("invalid or duplicate obsolete generated output name %q", name)
		}
		seen[name] = true
		dest := filepath.Join(dir, name)
		_, existed, err := outputMode(dest)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if existed {
			entries = append(entries, transactionFile{dest: dest, existed: true})
		}
	}
	rollback := func(cause error) error {
		var errs []error
		for i := len(entries) - 1; i >= 0; i-- {
			e := &entries[i]
			if e.applied && !e.existed {
				if err := os.Remove(e.dest); err != nil && !os.IsNotExist(err) {
					errs = append(errs, fmt.Errorf("remove newly applied %q: %w", e.dest, err))
				}
			}
			if e.backedUp {
				if e.applied {
					if err := os.Remove(e.dest); err != nil && !os.IsNotExist(err) {
						errs = append(errs, fmt.Errorf("remove replacement %q: %w", e.dest, err))
					}
				}
				if err := os.Rename(e.backup, e.dest); err != nil {
					errs = append(errs, fmt.Errorf("restore backup for %q: %w", e.dest, err))
				}
			}
			if err := os.Remove(e.stage); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("remove stage %q: %w", e.stage, err))
			}
		}
		return errors.Join(append([]error{cause}, errs...)...)
	}
	for i := range entries {
		e := &entries[i]
		if !e.existed {
			continue
		}
		backup, err := os.CreateTemp(dir, ".gogenconf-backup-"+filepath.Base(e.dest)+"-*")
		if err != nil {
			return rollback(fmt.Errorf("create exclusive backup for %q: %w", e.dest, err))
		}
		e.backup = backup.Name()
		if err := backup.Close(); err != nil {
			return rollback(fmt.Errorf("close backup for %q: %w", e.dest, err))
		}
		if err := os.Rename(e.dest, e.backup); err != nil {
			return rollback(fmt.Errorf("backup existing generated output %q: %w", e.dest, err))
		}
		e.backedUp = true
	}
	for i := range entries {
		e := &entries[i]
		if e.stage == "" { // obsolete file: its backup is the deletion.
			continue
		}
		if err := os.Rename(e.stage, e.dest); err != nil {
			return rollback(fmt.Errorf("apply generated output %q: %w", e.dest, err))
		}
		e.applied = true
	}
	for _, e := range entries {
		if e.backedUp {
			if err := os.Remove(e.backup); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("commit generated output transaction: remove backup %q: %w", e.backup, err)
			}
		}
	}
	return nil
}
