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

	"text/tabwriter"

	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/internal/nativeconfig"
)

// Root is a subcommand `gogenconf` -- Versioned declarative configuration language tooling
//
// gogenconf provides a native versioned configuration language, unresolved
// Document/Expr ASTs, and Go schemas with generated strongly typed bindings.
// Application schemas live in Go; this CLI validates language syntax only.
// Declarers remain unresolved until explicit application binding. No command
// reads environment/file sources mentioned inside a configuration value.
//
// Commands: generate, plan, list, doc, schema list, schema validate, schema plan, schema doc, format, validate, expr format, expr validate, template layout, template export, version.
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
	d, err := nativeconfig.Parse(bytes.NewReader(data))
	if err != nil {
		return err
	}
	out := []byte(nativeconfig.Format(d))
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
	_, err = nativeconfig.Parse(bytes.NewReader(b))
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
	_, err = fmt.Fprintln(os.Stdout, nativeconfig.FormatExpr(e))
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

// TemplateLayout is a subcommand `gogenconf template layout` -- Display built-in template layout
//
// Lists built-in code generation templates, their target generated file names,
// runtime capabilities, and descriptions.
//
// Examples:
//
//	gogenconf template layout
func TemplateLayout() error {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "TEMPLATE\tTARGET FILE\tCAPABILITY\tDESCRIPTION")
	for _, info := range codegen.BuiltinTemplateLayout() {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", info.Name, info.TargetFile, info.Capability, info.Description)
	}
	return w.Flush()
}

// TemplateExport is a subcommand `gogenconf template export` -- Export built-in templates to directory or txtar
//
// Exports the complete set of built-in code generation templates to a target
// directory or as a portable txtar archive.
//
// Examples:
//
//	gogenconf template export --output ./templates
//	gogenconf template export --as-txtar
//	gogenconf template export --as-txtar --output templates.txtar
//
// Flags:
//
//	output: --output (default: "") Destination directory or file path
//	asTxtar: --as-txtar (default: false) Export as portable txtar archive
func TemplateExport(output string, asTxtar bool) error {
	if asTxtar {
		data, err := codegen.ExportTemplatesTxtar()
		if err != nil {
			return err
		}
		if output == "" || output == "-" {
			_, err = os.Stdout.Write(data)
			return err
		}
		return os.WriteFile(output, data, 0644)
	}
	if output == "" || output == "-" {
		return fmt.Errorf("--output directory is required when not exporting as txtar")
	}
	return codegen.ExportTemplates(output)
}

func expr(s string) (nativeconfig.Expr, error) {
	if s == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(string(b))
	}
	return nativeconfig.ParseExpr(s)
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
