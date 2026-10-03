package codegen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"text/template"
)

//go:embed templates/*.gotmpl
var runtimeTemplatesFS embed.FS

var runtimeTemplates = template.Must(template.ParseFS(runtimeTemplatesFS, "templates/*.gotmpl"))

type templateData struct {
	Package string
}

// Generate executes the generation plan and produces deterministic formatted source files.
func (p *Plan) Generate() (FileSet, error) {
	var files FileSet

	render := func(tmplName, fileName string) error {
		tmpl := runtimeTemplates.Lookup(tmplName)
		if tmpl == nil {
			return fmt.Errorf("runtime template %s not found", tmplName)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, templateData{Package: p.Package}); err != nil {
			return fmt.Errorf("execute template %s: %w", tmplName, err)
		}
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("format generated source %s: %w\n%s", fileName, err, buf.String())
		}
		files = append(files, GeneratedFile{
			Name:    fileName,
			Content: formatted,
		})
		return nil
	}

	if p.HasCapability(CapabilityDocument) {
		if err := render("document.go.gotmpl", p.FilePrefix+"document_generated.go"); err != nil {
			return nil, err
		}
	}

	if p.HasCapability(CapabilityExpr) {
		if err := render("expr.go.gotmpl", p.FilePrefix+"expr_generated.go"); err != nil {
			return nil, err
		}
	}

	if p.HasCapability(CapabilityParser) {
		if err := render("parse.go.gotmpl", p.FilePrefix+"parse_generated.go"); err != nil {
			return nil, err
		}
	}

	if p.HasCapability(CapabilityFormatter) {
		if err := render("format.go.gotmpl", p.FilePrefix+"format_generated.go"); err != nil {
			return nil, err
		}
	}

	return files, nil
}

// GenerateRuntime creates a generation plan and produces the application-owned native runtime.
func GenerateRuntime(opts RuntimeOptions) (FileSet, error) {
	plan, err := PlanRuntime(opts)
	if err != nil {
		return nil, err
	}
	return plan.Generate()
}
