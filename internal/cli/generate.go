package cli

import (
	"fmt"
	"os"

	"github.com/arran4/gogenconf/codegen"
)

// Generate is a subcommand `gogenconf generate` -- Generate Go models and bindings from a schema
//
// Reads a canonical declarative schema input and emits strongly typed Go configuration
// models, binders, and optional dependency-free runtime code.
//
// Examples:
//
//	gogenconf generate --schema schema.conf --out ./config
//	gogenconf generate schema.conf
//	gogenconf generate --check --schema schema.conf --out ./config
//	gogenconf generate --clean --schema schema.conf --out ./config
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
//	out: --out (default: ".") Output directory for generated artifacts
//	pkg: --pkg (default: "") Target Go package name
//	check: --check (default: false) Check generated output without modifying files
//	clean: --clean (default: false) Remove obsolete generated files
//	runtime: --runtime (default: false) Emit standalone dependency-free runtime AST and parser
//	sample: --sample (default: false) Emit sample configuration document
func Generate(schema, file, out, pkg string, check, clean, runtime, sample bool) error {
	schemaPath := schema
	if schemaPath == "" {
		schemaPath = file
	}
	if schemaPath == "" {
		schemaPath = "schema.conf"
	}

	opts := codegen.ProjectOptions{
		SchemaFile:  schemaPath,
		OutputDir:   out,
		Package:     pkg,
		CheckOnly:   check,
		Clean:       clean,
		EmitRuntime: runtime,
		EmitSample:  sample,
	}

	plan, err := codegen.PlanProject(opts)
	if err != nil {
		return err
	}

	return plan.Execute(opts)
}

// Plan is a subcommand `gogenconf plan` -- Inspect the generation plan without writing files
//
// Reads a canonical declarative schema input and displays the resolved generation plan,
// including planned files, capabilities, and package layout.
//
// Examples:
//
//	gogenconf plan --schema schema.conf
//	gogenconf plan schema.conf --json
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
//	out: --out (default: ".") Target output directory
//	pkg: --pkg (default: "") Target Go package name
//	jsonOut: --json (default: false) Output machine-readable JSON plan
//	runtime: --runtime (default: false) Include standalone dependency-free runtime in plan
//	sample: --sample (default: false) Include sample document in plan
func Plan(schema, file, out, pkg string, jsonOut, runtime, sample bool) error {
	schemaPath := schema
	if schemaPath == "" {
		schemaPath = file
	}
	if schemaPath == "" {
		schemaPath = "schema.conf"
	}

	opts := codegen.ProjectOptions{
		SchemaFile:  schemaPath,
		OutputDir:   out,
		Package:     pkg,
		EmitRuntime: runtime,
		EmitSample:  sample,
	}

	plan, err := codegen.PlanProject(opts)
	if err != nil {
		return err
	}

	if jsonOut {
		data, err := plan.JSON()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(data))
		return err
	}

	_, err = fmt.Fprint(os.Stdout, plan.Summary())
	return err
}
