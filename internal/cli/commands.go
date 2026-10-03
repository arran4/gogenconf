// Package cli implements syntax-only tooling. It never constructs a resolver.
package cli

//go:generate go run ../cligen

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/arran4/gogenconf"
)

// Root is a subcommand `gogenconf` -- Versioned declarative configuration language tooling
//
// gogenconf provides a native versioned configuration language, unresolved
// Document/Expr ASTs, and Go schemas with generated strongly typed bindings.
// Application schemas live in Go; this CLI validates language syntax only.
// Declarers remain unresolved until explicit application binding. No command
// reads environment/file sources mentioned inside a configuration value.
//
// Commands: format, validate, expr format, expr validate, version.
// Use --help on any command for examples. Version reports build version/commit/date.
// Manual: https://github.com/arran4/gogenconf#readme
// Site: https://arran4.github.io/gogenconf/
func Root() {}

// Format is a subcommand `gogenconf format` -- Canonicalize a document without resolving sources
//
// Reads one file, or stdin when no file or '-' is supplied. Writes canonical
// native syntax to stdout. --check exits nonzero for a noncanonical document.
// --write replaces a regular file atomically, preserving its permissions;
// it refuses symlinks and stdin. --write and --check are mutually exclusive.
// Formatting preserves declarations, not secrecy: authored literals remain literals.
// Examples:
//
//	gogenconf format service.conf
//	cat service.conf | gogenconf format
//	gogenconf format --check service.conf
//	gogenconf format --write service.conf
//
// Flags:
//
//	file: @1 (default: "-") Input document or stdin
//	write: --write (default: false) Atomically replace the input file
//	check: --check (default: false) Check canonical form without writing
func Format(file string, write, check bool) error {
	if write && check {
		return fmt.Errorf("--write and --check are mutually exclusive")
	}
	if write && (file == "" || file == "-") {
		return fmt.Errorf("--write requires a regular file")
	}
	data, err := read(file)
	if err != nil {
		return err
	}
	d, err := gogenconf.Parse(bytes.NewReader(data))
	if err != nil {
		return err
	}
	out := []byte(gogenconf.Format(d))
	if check {
		if !bytes.Equal(data, out) {
			return fmt.Errorf("document is not canonical")
		}
		return nil
	}
	if write {
		return replace(file, out)
	}
	_, err = os.Stdout.Write(out)
	return err
}

// Validate is a subcommand `gogenconf validate` -- Validate native syntax, not application schema
//
// Checks document structure and expression syntax without resolution, migrations,
// or schema validation. Unknown sections/fields and future nonnegative versions
// are syntax-valid; only an application schema can judge their compatibility.
// Success is silent. Example: gogenconf validate service.conf
// Flags:
//
//	file: @1 (default: "-") Input document or stdin
func Validate(file string) error {
	b, err := read(file)
	if err != nil {
		return err
	}
	_, err = gogenconf.Parse(bytes.NewReader(b))
	return err
}

// ExprFormat is a subcommand `gogenconf expr format` -- Canonicalize one unresolved expression
//
// Quote the expression for your shell. With no argument or '-', read stdin.
// This command never opens declared files or reads declared environment variables.
// Example: gogenconf expr format 'from_file(from_env(CREDENTIAL_FILE))'
// Flags:
//
//	expression: @1 (default: "-") Expression text or '-' for stdin
func ExprFormat(expression string) error {
	e, err := expr(expression)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, gogenconf.FormatExpr(e))
	return err
}

// ExprValidate is a subcommand `gogenconf expr validate` -- Check expression grammar without resolving
//
// Unknown declarer names are syntactically valid; registration is application policy.
// Success is silent. Example: gogenconf expr validate 'from_json_file(from_env(CONFIG_FILE), .token)'
// Flags:
//
//	expression: @1 (default: "-") Expression text or '-' for stdin
func ExprValidate(expression string) error { _, err := expr(expression); return err }

func expr(s string) (gogenconf.Expr, error) {
	if s == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(string(b))
	}
	return gogenconf.ParseExpr(s)
}
func read(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}
func replace(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("--write requires a regular file, not a symlink")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".gogenconf-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(info.Mode().Perm()); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
