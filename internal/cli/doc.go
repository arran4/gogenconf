package cli

import (
	"github.com/arran4/gogenconf/codegen"
)

// Doc is a subcommand `gogenconf doc` -- Generate Markdown and man documentation from a schema
//
// Reads a canonical declarative schema input and generates reference documentation
// in Markdown, Unix man page (section 5), or canonical sample configuration.
//
// Examples:
//
//	gogenconf doc --schema schema.conf --out ./docs --markdown
//	gogenconf doc --schema schema.conf --out ./docs --man
//	gogenconf doc --schema schema.conf --out ./docs --sample
//	gogenconf doc --check --schema schema.conf --out ./docs --markdown
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
//	out: --out (default: ".") Output directory for generated documentation
//	markdown: --markdown (default: false) Emit Markdown configuration reference
//	man: --man (default: false) Emit section 5 Unix man page
//	sample: --sample (default: false) Emit canonical sample configuration document
//	check: --check (default: false) Verify documentation on disk is up to date without writing
//	app: --app (default: "") Application name for documentation titles
func Doc(schema, file, out string, markdown, man, sample, check bool, app string) error {
	schemaPath := schema
	if schemaPath == "" {
		schemaPath = file
	}
	if schemaPath == "" {
		schemaPath = "schema.conf"
	}

	emitMd := markdown
	emitMan := man
	emitSample := sample
	if !emitMd && !emitMan && !emitSample {
		emitMd = true
	}

	opts := codegen.DocProjectOptions{
		SchemaFile:   schemaPath,
		OutputDir:    out,
		AppName:      app,
		EmitMarkdown: emitMd,
		EmitMan:      emitMan,
		EmitSample:   emitSample,
		CheckOnly:    check,
	}

	plan, err := codegen.PlanDocs(opts)
	if err != nil {
		return err
	}

	return plan.Execute(opts)
}

// SchemaDoc is a subcommand `gogenconf schema doc` -- Generate Markdown and man documentation from a schema
//
// Alias for `gogenconf doc`.
//
// Flags:
//
//	schema: --schema (default: "") Path to declarative schema document
//	file: @1 (default: "") Positional schema document path
//	out: --out (default: ".") Output directory for generated documentation
//	markdown: --markdown (default: false) Emit Markdown configuration reference
//	man: --man (default: false) Emit section 5 Unix man page
//	sample: --sample (default: false) Emit canonical sample configuration document
//	check: --check (default: false) Verify documentation on disk is up to date without writing
//	app: --app (default: "") Application name for documentation titles
func SchemaDoc(schema, file, out string, markdown, man, sample, check bool, app string) error {
	return Doc(schema, file, out, markdown, man, sample, check, app)
}
