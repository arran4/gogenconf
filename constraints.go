package gogenconf

import (
	"fmt"
	"strconv"
	"strings"
)

// ConstraintKind identifies the type of policy constraint.
type ConstraintKind string

const (
	ConstraintKindMaxInt         ConstraintKind = "max_int"
	ConstraintKindMinInt         ConstraintKind = "min_int"
	ConstraintKindAllowedStrings ConstraintKind = "allowed_strings"
	ConstraintKindDeniedStrings  ConstraintKind = "denied_strings"
	ConstraintKindSystemOnly     ConstraintKind = "system_only"
)

// Constraint represents an immutable boundary enforced across composition layers.
type Constraint struct {
	Path       string         // e.g. "service.max_parallel" or "service.storage.path"
	Kind       ConstraintKind // max_int, min_int, allowed_strings, etc.
	IntValue   int64          // numeric threshold
	StringList []string       // allowed/denied strings
	EnforcedBy string         // identity of trusted layer or policy authority
	Sensitive  bool           // whether the field value is sensitive and should be redacted in errors
}

// PolicyEnvelope manages trusted constraints and enforces them against composed documents.
type PolicyEnvelope struct {
	Constraints      []Constraint
	SystemOnlyPaths  map[string]string // path -> enforcedBy
	SensitivePaths   map[string]bool   // path -> true
	CustomValidators []func(effective *Document) error
}

// NewPolicyEnvelope creates an empty policy envelope.
func NewPolicyEnvelope() *PolicyEnvelope {
	return &PolicyEnvelope{
		SystemOnlyPaths: make(map[string]string),
		SensitivePaths:  make(map[string]bool),
	}
}

// RequireMaxInt adds an upper bound constraint on an integer configuration path.
func (p *PolicyEnvelope) RequireMaxInt(path string, maxVal int64, enforcedBy string) {
	p.Constraints = append(p.Constraints, Constraint{
		Path:       path,
		Kind:       ConstraintKindMaxInt,
		IntValue:   maxVal,
		EnforcedBy: enforcedBy,
		Sensitive:  p.SensitivePaths[path],
	})
}

// RequireMinInt adds a lower bound constraint on an integer configuration path.
func (p *PolicyEnvelope) RequireMinInt(path string, minVal int64, enforcedBy string) {
	p.Constraints = append(p.Constraints, Constraint{
		Path:       path,
		Kind:       ConstraintKindMinInt,
		IntValue:   minVal,
		EnforcedBy: enforcedBy,
		Sensitive:  p.SensitivePaths[path],
	})
}

// RequireAllowedStrings restricts a scalar string or list of strings to an allowed whitelist.
func (p *PolicyEnvelope) RequireAllowedStrings(path string, allowed []string, enforcedBy string) {
	p.Constraints = append(p.Constraints, Constraint{
		Path:       path,
		Kind:       ConstraintKindAllowedStrings,
		StringList: allowed,
		EnforcedBy: enforcedBy,
		Sensitive:  p.SensitivePaths[path],
	})
}

// RequireDeniedStrings forbids matching strings in a scalar or list.
func (p *PolicyEnvelope) RequireDeniedStrings(path string, denied []string, enforcedBy string) {
	p.Constraints = append(p.Constraints, Constraint{
		Path:       path,
		Kind:       ConstraintKindDeniedStrings,
		StringList: denied,
		EnforcedBy: enforcedBy,
		Sensitive:  p.SensitivePaths[path],
	})
}

// MarkSystemOnly designates a field path as privileged; non-trusted layers must not configure it.
func (p *PolicyEnvelope) MarkSystemOnly(path string, enforcedBy string) {
	p.SystemOnlyPaths[path] = enforcedBy
	p.Constraints = append(p.Constraints, Constraint{
		Path:       path,
		Kind:       ConstraintKindSystemOnly,
		EnforcedBy: enforcedBy,
	})
}

// MarkSensitive flags a field path as containing sensitive credentials or tokens.
func (p *PolicyEnvelope) MarkSensitive(path string) {
	p.SensitivePaths[path] = true
	for i := range p.Constraints {
		if p.Constraints[i].Path == path {
			p.Constraints[i].Sensitive = true
		}
	}
}

// AddValidator registers a custom domain validator callback.
func (p *PolicyEnvelope) AddValidator(v func(effective *Document) error) {
	p.CustomValidators = append(p.CustomValidators, v)
}

// Enforce verifies that the effective document satisfies all policy constraints
// and that no untrusted layers authored system-only fields.
func (p *PolicyEnvelope) Enforce(effective *Document, layers []SourceLayer) error {
	if p == nil || effective == nil {
		return nil
	}

	// 1. Check for untrusted layers attempting to configure system-only fields
	if len(p.SystemOnlyPaths) > 0 {
		for _, layer := range layers {
			if layer.Trusted {
				continue
			}
			// Untrusted layer: inspect all entries to ensure no system-only field was authored
			for sysPath, enforcedBy := range p.SystemOnlyPaths {
				parts := strings.Split(sysPath, ".")
				val := layer.Document.PathValue(parts...)
				if val != nil {
					span := SpanOf(val)
					return &ConfigError{
						Stage:  StageValidate,
						Source: span,
						Err: fmt.Errorf("untrusted layer %q cannot configure system-only field %q (enforced by %s)",
							layer.Name, sysPath, enforcedBy),
					}
				}
			}
		}
	}

	// 2. Enforce scalar & collection constraints against the effective document
	for _, c := range p.Constraints {
		parts := strings.Split(c.Path, ".")
		val := effective.PathValue(parts...)
		if val == nil {
			continue // Omitted fields do not violate bounds unless marked required
		}
		span := SpanOf(val)

		switch c.Kind {
		case ConstraintKindMaxInt:
			intVal, err := extractInt(val)
			if err != nil {
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err:    fmt.Errorf("invalid integer value for %q: %w", c.Path, err),
				}
			}
			if intVal > c.IntValue {
				displayVal := strconv.FormatInt(intVal, 10)
				if c.Sensitive {
					displayVal = "[REDACTED]"
				}
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err: fmt.Errorf("value %s for %q exceeds trusted maximum limit %d (enforced by %s)",
						displayVal, c.Path, c.IntValue, c.EnforcedBy),
				}
			}

		case ConstraintKindMinInt:
			intVal, err := extractInt(val)
			if err != nil {
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err:    fmt.Errorf("invalid integer value for %q: %w", c.Path, err),
				}
			}
			if intVal < c.IntValue {
				displayVal := strconv.FormatInt(intVal, 10)
				if c.Sensitive {
					displayVal = "[REDACTED]"
				}
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err: fmt.Errorf("value %s for %q is below trusted minimum limit %d (enforced by %s)",
						displayVal, c.Path, c.IntValue, c.EnforcedBy),
				}
			}

		case ConstraintKindAllowedStrings:
			stringsToCheck, err := extractStrings(val)
			if err != nil {
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err:    fmt.Errorf("policy constraint %q validation failed: %w", c.Path, err),
				}
			}
			allowedMap := make(map[string]bool, len(c.StringList))
			for _, s := range c.StringList {
				allowedMap[s] = true
			}
			for _, s := range stringsToCheck {
				if !allowedMap[s] {
					displayVal := strconv.Quote(s)
					if c.Sensitive {
						displayVal = "[REDACTED]"
					}
					return &ConfigError{
						Stage:  StageValidate,
						Source: span,
						Err: fmt.Errorf("value %s for %q is not in trusted allowlist (enforced by %s)",
							displayVal, c.Path, c.EnforcedBy),
					}
				}
			}

		case ConstraintKindDeniedStrings:
			stringsToCheck, err := extractStrings(val)
			if err != nil {
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err:    fmt.Errorf("policy constraint %q validation failed: %w", c.Path, err),
				}
			}
			deniedMap := make(map[string]bool, len(c.StringList))
			for _, s := range c.StringList {
				deniedMap[s] = true
			}
			for _, s := range stringsToCheck {
				if deniedMap[s] {
					displayVal := strconv.Quote(s)
					if c.Sensitive {
						displayVal = "[REDACTED]"
					}
					return &ConfigError{
						Stage:  StageValidate,
						Source: span,
						Err: fmt.Errorf("value %s for %q is in trusted denylist (enforced by %s)",
							displayVal, c.Path, c.EnforcedBy),
					}
				}
			}
		}
	}

	// 3. Run application-owned custom validators
	for _, validator := range p.CustomValidators {
		if err := validator(effective); err != nil {
			return &ConfigError{
				Stage: StageValidate,
				Err:   fmt.Errorf("trusted policy validation failed: %w", err),
			}
		}
	}

	return nil
}

func extractInt(e Expr) (int64, error) {
	if lit, ok := e.(Literal); ok {
		return strconv.ParseInt(lit.Value, 10, 64)
	}
	if litPtr, ok := e.(*Literal); ok && litPtr != nil {
		return strconv.ParseInt(litPtr.Value, 10, 64)
	}
	return 0, fmt.Errorf("expected numeric literal, got %T", e)
}

func extractStrings(e Expr) ([]string, error) {
	var result []string
	switch v := e.(type) {
	case Literal:
		result = append(result, v.Value)
	case *Literal:
		if v != nil {
			result = append(result, v.Value)
		}
	case List:
		for _, item := range v.Items {
			sub, err := extractStrings(item)
			if err != nil {
				return nil, err
			}
			result = append(result, sub...)
		}
	case *List:
		if v != nil {
			for _, item := range v.Items {
				sub, err := extractStrings(item)
				if err != nil {
					return nil, err
				}
				result = append(result, sub...)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported expression shape %T in protected policy constraint", e)
	}
	return result, nil
}

// ComposeWithPolicy executes layer composition and verifies trusted policy constraints.
func ComposeWithPolicy(layers []SourceLayer, opts CompositionOptions, policy *PolicyEnvelope) (*Document, error) {
	effective, err := Compose(layers, opts)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		if err := policy.Enforce(effective, layers); err != nil {
			return nil, err
		}
	}
	return effective, nil
}
