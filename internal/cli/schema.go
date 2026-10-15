package cli

import (
	"fmt"
	"os"

	"github.com/arran4/gogenconf/codegen"
)

// List is a subcommand `gogenconf list` -- Inspect schema sections, fields, and types
//
// Reads a canonical declarative schema document and displays its sections, fields,
// Go mapping types, defaults, and policy flags in a structured outline.
//
// Examples:
//
//	gogenconf list --schema schema.conf
//	gogenconf list schema.conf --json
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
//	jsonOut: --json (default: false) Output machine-readable JSON format
func List(schema, file string, jsonOut bool) error {
	return SchemaList(schema, file, jsonOut)
}

// SchemaList is a subcommand `gogenconf schema list` -- Inspect schema sections, fields, and types
//
// Reads a canonical declarative schema document and displays its sections, fields,
// Go mapping types, defaults, and policy flags in a structured outline.
//
// Examples:
//
//	gogenconf schema list --schema schema.conf
//	gogenconf schema list schema.conf --json
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
//	jsonOut: --json (default: false) Output machine-readable JSON format
func SchemaList(schema, file string, jsonOut bool) error {
	schemaPath := schema
	if schemaPath == "" {
		schemaPath = file
	}
	if schemaPath == "" {
		schemaPath = "schema.conf"
	}

	info, err := codegen.InspectSchemaFile(schemaPath)
	if err != nil {
		return err
	}

	if jsonOut {
		data, err := info.JSON()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(data))
		return err
	}

	_, err = fmt.Fprint(os.Stdout, info.FormatText())
	return err
}

// SchemaValidate is a subcommand `gogenconf schema validate` -- Validate declarative schema syntax and generator constraints
//
// Validates a declarative schema document against generator constraints without
// emitting code. Checks for duplicate section/field names, invalid Go mappings,
// and unsupported resolution policies. Success is silent (exits 0).
//
// Examples:
//
//	gogenconf schema validate --schema schema.conf
//	gogenconf schema validate schema.conf
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
func SchemaValidate(schema, file string) error {
	schemaPath := schema
	if schemaPath == "" {
		schemaPath = file
	}
	if schemaPath == "" {
		schemaPath = "schema.conf"
	}

	_, err := codegen.LoadAndValidateSchema(schemaPath)
	return err
}

// SchemaPlan is a subcommand `gogenconf schema plan` -- Inspect the generation plan without writing files
//
// Reads a declarative schema document and displays the resolved multi-file generation plan.
// Alias for `gogenconf plan`.
//
// Examples:
//
//	gogenconf schema plan --schema schema.conf
//	gogenconf schema plan schema.conf --json
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
func SchemaPlan(schema, file, out, pkg string, jsonOut, runtime, sample bool) error {
	return Plan(schema, file, out, pkg, jsonOut, runtime, sample)
}
