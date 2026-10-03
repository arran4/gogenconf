// Package configmodel contains the reusable, unresolved configuration model.
package configmodel

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Expr is an unresolved value declaration. Parsing and formatting an Expr never
// performs I/O.
type Expr interface {
	expr()
}

// Literal is the implicit declaration used by a bare configuration value.
type Literal struct{ Value string }

func (Literal) expr() {}

// Call is a named declarer with recursively declared arguments.
type Call struct {
	Name string
	Args []Expr
}

func (Call) expr() {}

// ParseExpr parses a literal or a nested declarer expression.
func ParseExpr(input string) (Expr, error) {
	p := exprParser{input: strings.TrimSpace(input)}
	if p.input == "" {
		return Literal{}, nil
	}
	e, err := p.parseExpr(false)
	if err != nil {
		return nil, err
	}
	p.space()
	if p.pos != len(p.input) {
		return nil, fmt.Errorf("unexpected token at byte %d", p.pos)
	}
	return e, nil
}

type exprParser struct {
	input string
	pos   int
	depth int
}

func (p *exprParser) space() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *exprParser) parseExpr(argument bool) (Expr, error) {
	if p.depth > 64 {
		return nil, fmt.Errorf("expression nesting exceeds 64 levels")
	}
	p.space()
	if p.pos >= len(p.input) {
		return Literal{}, nil
	}
	if p.input[p.pos] == '"' {
		return p.quoted()
	}
	start := p.pos
	for p.pos < len(p.input) && isIdent(p.input[p.pos]) {
		p.pos++
	}
	if p.pos > start {
		name := p.input[start:p.pos]
		p.space()
		if p.pos < len(p.input) && p.input[p.pos] == '(' {
			if !identifier(name) {
				return nil, fmt.Errorf("invalid declarer identifier")
			}
			p.depth++
			defer func() { p.depth-- }()
			p.pos++
			var args []Expr
			p.space()
			if p.pos < len(p.input) && p.input[p.pos] == ')' {
				p.pos++
				return Call{Name: name}, nil
			}
			for {
				if p.pos >= len(p.input) || p.input[p.pos] == ',' || p.input[p.pos] == ')' {
					return nil, fmt.Errorf("missing call argument")
				}
				a, err := p.parseExpr(true)
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				p.space()
				if p.pos >= len(p.input) {
					return nil, fmt.Errorf("unterminated call")
				}
				if p.input[p.pos] == ')' {
					p.pos++
					return Call{Name: name, Args: args}, nil
				}
				if p.input[p.pos] != ',' {
					return nil, fmt.Errorf("expected comma or closing parenthesis at byte %d", p.pos)
				}
				p.pos++
				p.space()
			}
		}
		// It is a literal, not a call. Consume only this argument, leaving an
		// enclosing comma or closing parenthesis for its caller.
		p.pos = start
	}
	// A bare literal owns the remaining top-level source, or up to an enclosing
	// call delimiter when it is an argument.
	valueStart := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if argument && (c == ',' || c == ')') {
			break
		}
		if c == '(' || c == ')' || c == '"' {
			return nil, fmt.Errorf("quote literal punctuation at byte %d", p.pos)
		}
		p.pos++
	}
	v := strings.TrimSpace(p.input[valueStart:p.pos])
	return Literal{Value: v}, nil
}

func (p *exprParser) quoted() (Expr, error) {
	start := p.pos
	p.pos++
	for p.pos < len(p.input) {
		if p.input[p.pos] == '\\' {
			p.pos += 2
			continue
		}
		if p.input[p.pos] == '"' {
			p.pos++
			raw := p.input[start:p.pos]
			v, err := strconv.Unquote(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid quoted escape at byte %d", start)
			}
			return Literal{Value: v}, nil
		}
		p.pos++
	}
	return nil, fmt.Errorf("unterminated quoted literal")
}

func isIdent(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

// FormatExpr renders an expression canonically without resolving it.
func FormatExpr(e Expr) string {
	switch e := e.(type) {
	case Literal:
		if bareLiteral(e.Value) {
			return e.Value
		}
		return strconv.Quote(e.Value)
	case *Literal:
		return FormatExpr(*e)
	case Call:
		args := make([]string, len(e.Args))
		for i, a := range e.Args {
			args[i] = FormatExpr(a)
		}
		return e.Name + "(" + strings.Join(args, ", ") + ")"
	case *Call:
		return FormatExpr(*e)
	default:
		return ""
	}
}

func bareLiteral(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == ',' || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}
