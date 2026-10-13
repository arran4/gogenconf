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
	HasEnv  bool
	HasFile bool
	HasJSON bool
}

// Generate executes the generation plan and produces deterministic formatted source files.
func (p *Plan) Generate() (FileSet, error) {
	var files FileSet

	tmplSet := runtimeTemplates
	if len(p.Overlays) > 0 {
		var err error
		tmplSet, err = BuildTemplateSet(p.Overlays)
		if err != nil {
			return nil, fmt.Errorf("build template set: %w", err)
		}
	}

	render := func(tmplName, fileName string) error {
		tmpl := tmplSet.Lookup(tmplName)
		if tmpl == nil {
			return fmt.Errorf("runtime template %s not found", tmplName)
		}
		var buf bytes.Buffer
		td := templateData{
			Package: p.Package,
			HasEnv:  p.HasDeclarer("from_env"),
			HasFile: p.HasDeclarer("from_file"),
			HasJSON: p.HasDeclarer("from_json_file"),
		}
		if err := tmpl.Execute(&buf, td); err != nil {
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

	if p.HasCapability(CapabilityResolver) {
		if err := render("resolve.go.gotmpl", p.FilePrefix+"resolve_generated.go"); err != nil {
			return nil, err
		}
	}

	if p.HasCapability(CapabilityProvider) {
		if err := render("provider.go.gotmpl", p.FilePrefix+"provider_generated.go"); err != nil {
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
