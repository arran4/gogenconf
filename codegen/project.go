package codegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	m "github.com/arran4/gogenconf"
)

// ProjectOptions configures full project generation from a declarative schema input.
type ProjectOptions struct {
	// SchemaFile is the path to the canonical schema input file (optional if Schema is provided).
	SchemaFile string

	// Schema is the parsed canonical Schema (optional if SchemaFile is provided).
	Schema *m.Schema

	// OutputDir is the target destination directory (defaults to ".").
	OutputDir string

	// Package is the destination Go package name. If empty, it is inferred from OutputDir or Schema identity.
	Package string

	// FilePrefix is prepended to generated source file names (defaults to "config_").
	FilePrefix string

	// SkipModel disables emitting Go configuration struct models.
	SkipModel bool

	// SkipBinder disables emitting Go configuration resolution binders.
	SkipBinder bool

	// EmitRuntime controls whether the application-owned, dependency-free native runtime AST/parser is emitted.
	EmitRuntime bool

	// EmitSample controls whether a sample configuration document is emitted (defaults to false).
	EmitSample bool

	// SampleFile is the file name for the sample configuration document (defaults to "sample.conf").
	SampleFile string

	// CheckOnly verifies that existing files on disk match planned output without modifying files.
	CheckOnly bool

	// Clean removes obsolete generated files matching the neutral gogenconf header.
	Clean bool

	// LibraryImport overrides the configuration library import in generated code.
	LibraryImport string
}

// ProjectPlan captures the deterministic, multi-file output plan resolved before touching disk.
type ProjectPlan struct {
	Package       string            `json:"package"`
	OutputDir     string            `json:"output_dir"`
	Capabilities  []string          `json:"capabilities"`
	Files         FileSet           `json:"files"`
	ObsoleteFiles []string          `json:"obsolete_files,omitempty"`
	Reasons       map[string]string `json:"reasons,omitempty"`
}

// PlanProject validates options and input schemas, resolving an in-memory generation plan.
func PlanProject(opts ProjectOptions) (*ProjectPlan, error) {
	s := opts.Schema
	if s == nil {
		if opts.SchemaFile == "" {
			return nil, fmt.Errorf("schema file or schema object is required")
		}
		var err error
		if opts.SchemaFile == "-" {
			s, err = m.ParseSchemaNamed(os.Stdin, "stdin")
			if err != nil {
				return nil, fmt.Errorf("parse schema from stdin: %w", err)
			}
		} else {
			data, err := os.ReadFile(opts.SchemaFile)
			if err != nil {
				return nil, fmt.Errorf("read schema file %s: %w", opts.SchemaFile, err)
			}
			s, err = m.ParseSchemaNamed(bytes.NewReader(data), opts.SchemaFile)
			if err != nil {
				return nil, fmt.Errorf("parse schema %s: %w", opts.SchemaFile, err)
			}
		}
	}

	if err := Validate(*s); err != nil {
		return nil, fmt.Errorf("validate schema: %w", err)
	}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = "."
	}

	pkg := opts.Package
	if pkg == "" {
		if outDir != "." && outDir != "" {
			pkg = cleanPackageName(filepath.Base(outDir))
		}
		if pkg == "" && s.Identity != "" {
			cleanIdent := strings.TrimSuffix(s.Identity, "-schema")
			pkg = cleanPackageName(cleanIdent)
		}
		if pkg == "" {
			pkg = "config"
		}
	}
	if !token.IsIdentifier(pkg) || pkg == "_" {
		return nil, fmt.Errorf("invalid package name %q", pkg)
	}

	prefix := opts.FilePrefix
	if prefix == "" {
		prefix = "config_"
	}

	plan := &ProjectPlan{
		Package:   pkg,
		OutputDir: outDir,
		Reasons:   make(map[string]string),
	}

	emitModel := !opts.SkipModel
	emitBinder := !opts.SkipBinder

	if emitModel || emitBinder {
		plan.Capabilities = append(plan.Capabilities, "model_and_binder")
		genOpts := Options{
			Package:       pkg,
			LibraryImport: opts.LibraryImport,
		}
		content, err := generate(*s, genOpts, emitModel, emitBinder)
		if err != nil {
			return nil, fmt.Errorf("generate model/binder: %w", err)
		}
		fileName := prefix + "generated.go"
		plan.Files = append(plan.Files, GeneratedFile{
			Name:    fileName,
			Content: content,
		})
		plan.Reasons[fileName] = "strongly typed Go model structs and resolution bindings"
	}

	emitRuntime := opts.EmitRuntime
	if !opts.EmitRuntime && opts.LibraryImport == "" {
		emitRuntime = true
	}

	if emitRuntime {
		plan.Capabilities = append(plan.Capabilities, "dependency_free_runtime")
		runtimeFiles, err := GenerateRuntime(RuntimeOptions{
			Package:    pkg,
			FilePrefix: prefix,
		})
		if err != nil {
			return nil, fmt.Errorf("generate runtime: %w", err)
		}
		for _, rf := range runtimeFiles {
			plan.Files = append(plan.Files, rf)
			plan.Reasons[rf.Name] = "dependency-free native configuration AST parser and formatter"
		}
	}

	if opts.EmitSample {
		plan.Capabilities = append(plan.Capabilities, "sample_document")
		sampleFile := opts.SampleFile
		if sampleFile == "" {
			sampleFile = "sample.conf"
		}
		sampleDoc := s.Sample()
		plan.Files = append(plan.Files, GeneratedFile{
			Name:    sampleFile,
			Content: []byte(m.Format(sampleDoc)),
		})
		plan.Reasons[sampleFile] = "illustrative sample configuration document generated from schema metadata"
	}

	// Scan output directory for obsolete generated files if output directory exists.
	// Only files matching the gogenconf generated file convention are considered.
	if entries, err := os.ReadDir(outDir); err == nil {
		plannedNames := make(map[string]bool, len(plan.Files))
		for _, f := range plan.Files {
			plannedNames[f.Name] = true
		}

		for _, e := range entries {
			if e.IsDir() || plannedNames[e.Name()] {
				continue
			}
			name := e.Name()
			// Only consider files adhering to the generated naming convention
			isGeneratedName := strings.HasSuffix(name, "_generated.go") || (opts.SampleFile != "" && name == opts.SampleFile)
			if !isGeneratedName {
				continue
			}
			filePath := filepath.Join(outDir, name)
			if isGeneratedByGogenconf(filePath) {
				plan.ObsoleteFiles = append(plan.ObsoleteFiles, name)
			}
		}
	}

	return plan, nil
}

// Execute applies the plan according to the options: verifying in CheckOnly mode,
// or atomically writing files and removing obsolete ones.
func (p *ProjectPlan) Execute(opts ProjectOptions) error {
	outDir := p.OutputDir
	if outDir == "" {
		outDir = "."
	}

	if opts.CheckOnly {
		for _, f := range p.Files {
			targetPath := filepath.Join(outDir, f.Name)
			diskContent, err := os.ReadFile(targetPath)
			if err != nil {
				return fmt.Errorf("missing generated file %q: %w", f.Name, err)
			}
			if !bytes.Equal(diskContent, f.Content) {
				return fmt.Errorf("generated file %q is stale / out of date", f.Name)
			}
		}
		if opts.Clean && len(p.ObsoleteFiles) > 0 {
			return fmt.Errorf("obsolete generated file(s) present on disk: %s", strings.Join(p.ObsoleteFiles, ", "))
		}
		return nil
	}

	obsolete := []string(nil)
	if opts.Clean {
		obsolete = p.ObsoleteFiles
	}
	return p.Files.writeTransaction(outDir, obsolete)
}

// Summary returns a human-readable summary of the generation plan.
func (p *ProjectPlan) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generation Plan:\n")
	fmt.Fprintf(&b, "  Target Package:   %s\n", p.Package)
	fmt.Fprintf(&b, "  Output Directory: %s\n", p.OutputDir)
	fmt.Fprintf(&b, "  Capabilities:     %s\n\n", strings.Join(p.Capabilities, ", "))
	fmt.Fprintf(&b, "Planned Files (%d):\n", len(p.Files))
	for _, f := range p.Files {
		reason := p.Reasons[f.Name]
		if reason != "" {
			fmt.Fprintf(&b, "  + %s (%d bytes) - %s\n", f.Name, len(f.Content), reason)
		} else {
			fmt.Fprintf(&b, "  + %s (%d bytes)\n", f.Name, len(f.Content))
		}
	}
	if len(p.ObsoleteFiles) > 0 {
		fmt.Fprintf(&b, "\nObsolete Files to Remove (%d):\n", len(p.ObsoleteFiles))
		for _, obs := range p.ObsoleteFiles {
			fmt.Fprintf(&b, "  - %s\n", obs)
		}
	}
	return b.String()
}

// JSON returns a JSON serialization of the generation plan.
func (p *ProjectPlan) JSON() ([]byte, error) {
	type fileInfo struct {
		Name   string `json:"name"`
		Size   int    `json:"size"`
		Reason string `json:"reason,omitempty"`
	}
	var fileList []fileInfo
	for _, f := range p.Files {
		fileList = append(fileList, fileInfo{
			Name:   f.Name,
			Size:   len(f.Content),
			Reason: p.Reasons[f.Name],
		})
	}
	payload := struct {
		Package       string     `json:"package"`
		OutputDir     string     `json:"output_dir"`
		Capabilities  []string   `json:"capabilities"`
		Files         []fileInfo `json:"files"`
		ObsoleteFiles []string   `json:"obsolete_files,omitempty"`
	}{
		Package:       p.Package,
		OutputDir:     p.OutputDir,
		Capabilities:  p.Capabilities,
		Files:         fileList,
		ObsoleteFiles: p.ObsoleteFiles,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func isGeneratedByGogenconf(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	return bytes.Contains(buf[:n], []byte("// Code generated by gogenconf. DO NOT EDIT.")) ||
		bytes.Contains(buf[:n], []byte("# Code generated by gogenconf. DO NOT EDIT.")) ||
		bytes.Contains(buf[:n], []byte("documentation is schema-managed."))
}

func cleanPackageName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	res := b.String()
	if res == "" || !token.IsIdentifier(res) || res == "_" {
		return ""
	}
	return res
}
