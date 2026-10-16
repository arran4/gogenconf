package codegen

import (
	"fmt"
	"go/token"

	m "github.com/arran4/gogenconf"
)

// Capability represents a modular feature that can be emitted into generated code.
type Capability string

const (
	CapabilityDocument  Capability = "document"
	CapabilityExpr      Capability = "expr"
	CapabilityParser    Capability = "parser"
	CapabilityFormatter Capability = "formatter"
	CapabilityResolver  Capability = "resolver"
	CapabilityProvider  Capability = "provider"
)

// DefaultRuntimeCapabilities returns the canonical list of native runtime capabilities.
func DefaultRuntimeCapabilities() []Capability {
	return []Capability{
		CapabilityDocument,
		CapabilityExpr,
		CapabilityParser,
		CapabilityFormatter,
	}
}

// FullRuntimeCapabilities returns all available native runtime capabilities.
func FullRuntimeCapabilities() []Capability {
	return []Capability{
		CapabilityDocument,
		CapabilityExpr,
		CapabilityParser,
		CapabilityFormatter,
		CapabilityResolver,
		CapabilityProvider,
	}
}

// RuntimeOptions configures generation of an application-owned native config runtime.
type RuntimeOptions struct {
	// Package is the destination package name (e.g. "config"). Required.
	Package string

	// FilePrefix is the prefix prepended to generated runtime file names.
	// Defaults to "config_".
	FilePrefix string

	// Capabilities selects the runtime features to emit.
	// When empty, DefaultRuntimeCapabilities() is used.
	Capabilities []Capability

	// Declarers selects which standard declarers to generate (e.g. "from_env", "from_file", "from_json_file").
	// When empty and CapabilityResolver is enabled, all standard declarers are included unless
	// configured via PlanFromSchema.
	Declarers []string

	// Overlays specifies an optional ordered list of template overlays to apply.
	Overlays []Overlay
}

// Plan models the verified generation requirements and scheduled source emitters.
type Plan struct {
	Package      string
	FilePrefix   string
	Capabilities map[Capability]bool
	Declarers    map[string]bool
	Overlays     []Overlay
}

// PlanRuntime validates options and constructs a generation Plan.
func PlanRuntime(opts RuntimeOptions) (*Plan, error) {
	if !token.IsIdentifier(opts.Package) || opts.Package == "_" {
		return nil, fmt.Errorf("invalid package name %q", opts.Package)
	}

	prefix := opts.FilePrefix
	if prefix == "" {
		prefix = "config_"
	}

	caps := opts.Capabilities
	if len(caps) == 0 {
		caps = DefaultRuntimeCapabilities()
	}

	capMap := make(map[Capability]bool, len(caps))
	for _, c := range caps {
		switch c {
		case CapabilityDocument, CapabilityExpr, CapabilityParser, CapabilityFormatter, CapabilityResolver, CapabilityProvider:
			capMap[c] = true
		default:
			return nil, fmt.Errorf("unsupported capability %q", c)
		}
	}

	// Resolve dependencies:
	if capMap[CapabilityProvider] {
		capMap[CapabilityResolver] = true
	}
	if capMap[CapabilityResolver] {
		capMap[CapabilityDocument] = true
		capMap[CapabilityExpr] = true
	}
	if capMap[CapabilityParser] {
		capMap[CapabilityDocument] = true
		capMap[CapabilityExpr] = true
	}
	if capMap[CapabilityFormatter] {
		capMap[CapabilityDocument] = true
		capMap[CapabilityExpr] = true
	}
	if capMap[CapabilityDocument] {
		capMap[CapabilityExpr] = true
	}

	declMap := make(map[string]bool)
	if opts.Declarers != nil {
		for _, d := range opts.Declarers {
			declMap[d] = true
		}
	} else if capMap[CapabilityResolver] {
		// Default to all standard declarers if resolver is enabled but declarers not explicitly filtered
		declMap["from_env"] = true
		declMap["from_file"] = true
		declMap["from_json_file"] = true
	}

	return &Plan{
		Package:      opts.Package,
		FilePrefix:   prefix,
		Capabilities: capMap,
		Declarers:    declMap,
		Overlays:     opts.Overlays,
	}, nil
}

// PlanFromSchema analyzes a Schema and constructs a tailored, feature-selected Plan.
func PlanFromSchema(s m.Schema, opts RuntimeOptions) (*Plan, error) {
	caps := []Capability{
		CapabilityDocument,
		CapabilityExpr,
		CapabilityParser,
		CapabilityFormatter,
		CapabilityResolver,
	}

	hasProvider := false
	declarers := make(map[string]bool)

	var inspectExpr func(e m.Expr)
	inspectExpr = func(e m.Expr) {
		switch v := e.(type) {
		case m.Call:
			switch v.Name {
			case "from_env":
				declarers["from_env"] = true
			case "from_file":
				declarers["from_file"] = true
			case "from_json_file":
				declarers["from_json_file"] = true
			case "from_env_file":
				declarers["from_env"] = true
				declarers["from_file"] = true
			}
			for _, arg := range v.Args {
				inspectExpr(arg)
			}
		case *m.Call:
			if v != nil {
				inspectExpr(*v)
			}
		}
	}

	for _, sec := range s.Sections {
		for _, f := range sec.Fields {
			if f.Policy == m.ProviderBacked {
				hasProvider = true
			}
			if f.Environment != "" {
				declarers["from_env"] = true
			}
			if f.Default != nil {
				inspectExpr(f.Default)
			}
			for _, ex := range f.Examples {
				if ex != nil {
					inspectExpr(ex)
				}
			}
		}
	}

	if hasProvider {
		caps = append(caps, CapabilityProvider)
	}

	if len(opts.Capabilities) > 0 {
		caps = opts.Capabilities
	}

	opts.Capabilities = caps

	if opts.Declarers == nil {
		decls := make([]string, 0, len(declarers))
		for d := range declarers {
			decls = append(decls, d)
		}
		opts.Declarers = decls
	}

	return PlanRuntime(opts)
}

// NewPlan is an alias for PlanRuntime.
func NewPlan(opts RuntimeOptions) (*Plan, error) {
	return PlanRuntime(opts)
}

// HasCapability reports whether capability c is enabled in the plan.
func (p *Plan) HasCapability(c Capability) bool {
	return p.Capabilities[c]
}

// HasDeclarer reports whether declarer name is enabled in the plan.
func (p *Plan) HasDeclarer(name string) bool {
	return p.Declarers[name]
}
