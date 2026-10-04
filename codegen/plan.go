package codegen

import (
	"fmt"
	"go/token"
)

// Capability represents a modular feature that can be emitted into generated code.
type Capability string

const (
	CapabilityDocument  Capability = "document"
	CapabilityExpr      Capability = "expr"
	CapabilityParser    Capability = "parser"
	CapabilityFormatter Capability = "formatter"
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
}

// Plan models the verified generation requirements and scheduled source emitters.
type Plan struct {
	Package      string
	FilePrefix   string
	Capabilities map[Capability]bool
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
		case CapabilityDocument, CapabilityExpr, CapabilityParser, CapabilityFormatter:
			capMap[c] = true
		default:
			return nil, fmt.Errorf("unsupported capability %q", c)
		}
	}

	// Resolve dependencies: parser and formatter require document and expr ASTs.
	if capMap[CapabilityParser] {
		capMap[CapabilityDocument] = true
		capMap[CapabilityExpr] = true
	}
	if capMap[CapabilityFormatter] {
		capMap[CapabilityDocument] = true
		capMap[CapabilityExpr] = true
	}
	// Document AST contains Entry which holds Value Expr, so Document requires Expr.
	if capMap[CapabilityDocument] {
		capMap[CapabilityExpr] = true
	}

	return &Plan{
		Package:      opts.Package,
		FilePrefix:   prefix,
		Capabilities: capMap,
	}, nil
}

// NewPlan is an alias for PlanRuntime.
func NewPlan(opts RuntimeOptions) (*Plan, error) {
	return PlanRuntime(opts)
}

// HasCapability reports whether capability c is enabled in the plan.
func (p *Plan) HasCapability(c Capability) bool {
	return p.Capabilities[c]
}
