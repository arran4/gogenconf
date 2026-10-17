package codegen

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"golang.org/x/tools/txtar"
)

// TemplateInfo describes a built-in template and its target generated artifact.
type TemplateInfo struct {
	Name        string     `json:"name"`
	TargetFile  string     `json:"target_file"`
	Capability  Capability `json:"capability"`
	Description string     `json:"description"`
}

var knownTemplateMetadata = map[string]TemplateInfo{
	"document.go.gotmpl": {
		Name:        "document.go.gotmpl",
		TargetFile:  "document_generated.go",
		Capability:  CapabilityDocument,
		Description: "AST document, sections, items, and comment model",
	},
	"expr.go.gotmpl": {
		Name:        "expr.go.gotmpl",
		TargetFile:  "expr_generated.go",
		Capability:  CapabilityExpr,
		Description: "Expression AST nodes, literals, and calls",
	},
	"parse.go.gotmpl": {
		Name:        "parse.go.gotmpl",
		TargetFile:  "parse_generated.go",
		Capability:  CapabilityParser,
		Description: "Native configuration parser and scanner",
	},
	"format.go.gotmpl": {
		Name:        "format.go.gotmpl",
		TargetFile:  "format_generated.go",
		Capability:  CapabilityFormatter,
		Description: "Deterministic native configuration and expression formatter",
	},
	"provider.go.gotmpl": {
		Name:        "provider.go.gotmpl",
		TargetFile:  "provider_generated.go",
		Capability:  Capability("provider"),
		Description: "Application-owned resolvers, lookup environments, and provider registries",
	},
	"resolve.go.gotmpl": {
		Name:        "resolve.go.gotmpl",
		TargetFile:  "resolve_generated.go",
		Capability:  Capability("resolve"),
		Description: "Static expression evaluation and value resolution runtime",
	},
}

// BuiltinTemplateLayout returns the canonical metadata layout of built-in templates.
func BuiltinTemplateLayout() []TemplateInfo {
	entries, err := runtimeTemplatesFS.ReadDir("templates")
	if err != nil {
		return nil
	}
	var layout []TemplateInfo
	for _, entry := range entries {
		name := entry.Name()
		if info, ok := knownTemplateMetadata[name]; ok {
			layout = append(layout, info)
		} else {
			layout = append(layout, TemplateInfo{
				Name:        name,
				TargetFile:  strings.TrimSuffix(name, ".gotmpl"),
				Description: "Built-in code generation template",
			})
		}
	}
	sort.Slice(layout, func(i, j int) bool {
		return layout[i].Name < layout[j].Name
	})
	return layout
}

// Overlay modifies or replaces template content before execution.
type Overlay interface {
	Apply(target map[string]string) error
}

// FileOverlay replaces a single named template.
type FileOverlay struct {
	Name    string
	Content string
}

func (f FileOverlay) Apply(target map[string]string) error {
	clean := path.Clean(f.Name)
	if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".." {
		return fmt.Errorf("invalid template path %q", f.Name)
	}
	base := path.Base(clean)
	if !isKnownTemplate(base) {
		return fmt.Errorf("unknown template target %q", f.Name)
	}
	target[base] = f.Content
	return nil
}

// OverlayFile creates a single named template replacement overlay.
func OverlayFile(name, content string) Overlay {
	return FileOverlay{Name: name, Content: content}
}

// FSOverlay applies template files from an fs.FS abstraction.
type FSOverlay struct {
	FS fs.FS
}

func (f FSOverlay) Apply(target map[string]string) error {
	return fs.WalkDir(f.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		clean := path.Clean(p)
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".." {
			return fmt.Errorf("invalid template path %q", p)
		}
		base := path.Base(clean)
		if !strings.HasSuffix(base, ".gotmpl") {
			return nil
		}
		if !isKnownTemplate(base) {
			return fmt.Errorf("unknown template target %q", base)
		}
		content, err := fs.ReadFile(f.FS, p)
		if err != nil {
			return fmt.Errorf("read overlay template %q: %w", p, err)
		}
		target[base] = string(content)
		return nil
	})
}

// OverlayFS creates an overlay from an fs.FS abstraction.
func OverlayFS(fsys fs.FS) Overlay {
	return FSOverlay{FS: fsys}
}

// OverlayDir creates an overlay from a local directory path.
func OverlayDir(dir string) (Overlay, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("overlay directory %q: %w", dir, err)
	}
	return FSOverlay{FS: os.DirFS(dir)}, nil
}

// TxtarOverlay applies template replacements extracted from a txtar archive.
type TxtarOverlay struct {
	Data []byte
}

func (t TxtarOverlay) Apply(target map[string]string) error {
	ar := txtar.Parse(t.Data)
	for _, f := range ar.Files {
		clean := path.Clean(f.Name)
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".." {
			return fmt.Errorf("invalid template path %q in txtar archive", f.Name)
		}
		base := path.Base(clean)
		if !strings.HasSuffix(base, ".gotmpl") {
			continue
		}
		if !isKnownTemplate(base) {
			return fmt.Errorf("unknown template target %q", base)
		}
		target[base] = string(f.Data)
	}
	return nil
}

// OverlayTxtar creates an overlay from a txtar archive bytes.
func OverlayTxtar(data []byte) Overlay {
	return TxtarOverlay{Data: data}
}

func isKnownTemplate(name string) bool {
	if _, ok := knownTemplateMetadata[name]; ok {
		return true
	}
	_, err := runtimeTemplatesFS.ReadFile("templates/" + name)
	return err == nil
}

// ExportTemplates writes the complete built-in template set to destDir.
func ExportTemplates(destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create export dir: %w", err)
	}
	entries, err := runtimeTemplatesFS.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("read builtin templates: %w", err)
	}
	for _, entry := range entries {
		data, err := runtimeTemplatesFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read template %s: %w", entry.Name(), err)
		}
		outPath := filepath.Join(destDir, entry.Name())
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return fmt.Errorf("write template %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// ExportTemplatesTxtar returns a portable txtar archive containing all built-in templates.
func ExportTemplatesTxtar() ([]byte, error) {
	entries, err := runtimeTemplatesFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("read builtin templates: %w", err)
	}
	var ar txtar.Archive
	for _, entry := range entries {
		data, err := runtimeTemplatesFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", entry.Name(), err)
		}
		ar.Files = append(ar.Files, txtar.File{
			Name: entry.Name(),
			Data: data,
		})
	}
	return txtar.Format(&ar), nil
}

// BuildTemplateSet combines built-in templates with ordered overlays.
func BuildTemplateSet(overlays []Overlay) (*template.Template, error) {
	tmpls := make(map[string]string)
	entries, err := runtimeTemplatesFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("read builtin templates: %w", err)
	}
	for _, entry := range entries {
		data, err := runtimeTemplatesFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", entry.Name(), err)
		}
		tmpls[entry.Name()] = string(data)
	}

	for _, ov := range overlays {
		if ov == nil {
			continue
		}
		if err := ov.Apply(tmpls); err != nil {
			return nil, err
		}
	}

	rootTmpl := template.New("runtime")
	var names []string
	for k := range tmpls {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		_, err := rootTmpl.New(name).Parse(tmpls[name])
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
	}
	return rootTmpl, nil
}
