// Package gogenconf contains the reusable, unresolved configuration model.
package gogenconf

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
type Literal struct {
	Value  string
	Span   Span
	Origin Origin
}

func (Literal) expr() {}

// Call is a named declarer with recursively declared arguments.
type Call struct {
	Name   string
	Args   []Expr
	Span   Span
	Origin Origin
}

func (Call) expr() {}

func SpanOf(e Expr) Span {
	switch v := e.(type) {
	case Literal:
		return v.Span
	case *Literal:
		if v != nil {
			return v.Span
		}
	case Call:
		return v.Span
	case *Call:
		if v != nil {
			return v.Span
		}
	}
	return Span{}
}

func OriginOf(e Expr) Origin {
	switch v := e.(type) {
	case Literal:
		return v.Origin
	case *Literal:
		if v != nil {
			return v.Origin
		}
	case Call:
		return v.Origin
	case *Call:
		if v != nil {
			return v.Origin
		}
	}
	return OriginAuthored
}

func WithSpan(e Expr, s Span) Expr {
	switch v := e.(type) {
	case Literal:
		v.Span = s
		return v
	case *Literal:
		if v != nil {
			c := *v
			c.Span = s
			return &c
		}
	case Call:
		v.Span = s
		return v
	case *Call:
		if v != nil {
			c := *v
			c.Span = s
			return &c
		}
	}
	return e
}

func WithOrigin(e Expr, o Origin) Expr {
	switch v := e.(type) {
	case Literal:
		v.Origin = o
		return v
	case *Literal:
		if v != nil {
			c := *v
			c.Origin = o
			return &c
		}
	case Call:
		v.Origin = o
		return v
	case *Call:
		if v != nil {
			c := *v
			c.Origin = o
			return &c
		}
	}
	return e
}

// ParseExpr parses a literal or a nested declarer expression.
func ParseExpr(input string) (Expr, error) {
	return ParseExprWithSpan(input, Span{})
}

// ParseExprWithSpan parses a literal or a nested declarer expression, attaching source span metadata.
func ParseExprWithSpan(input string, base Span) (Expr, error) {
	trimmed := strings.TrimSpace(input)
	leadingSpace := len(input) - len(strings.TrimLeft(input, " \t\r\n"))
	startCol := base.Column
	if startCol > 0 {
		startCol += leadingSpace
	}
	startOffset := base.Offset
	if startOffset > 0 || base.Line > 0 {
		startOffset += leadingSpace
	}
	p := exprParser{
		input:       trimmed,
		baseSource:  base.Source,
		baseLine:    base.Line,
		startCol:    startCol,
		startOffset: startOffset,
	}
	if p.input == "" {
		return Literal{
			Span: Span{
				Source: base.Source,
				Line:   base.Line,
				Column: startCol,
				Offset: startOffset,
			},
			Origin: OriginAuthored,
		}, nil
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
	input       string
	pos         int
	depth       int
	baseSource  string
	baseLine    int
	startCol    int
	startOffset int
}

func (p *exprParser) spanAt(startPos int) Span {
	col := 0
	if p.startCol > 0 {
		col = p.startCol + startPos
	}
	off := 0
	if p.startOffset > 0 || p.baseLine > 0 {
		off = p.startOffset + startPos
	}
	return Span{
		Source: p.baseSource,
		Line:   p.baseLine,
		Column: col,
		Offset: off,
	}
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
		return Literal{Span: p.spanAt(p.pos), Origin: OriginAuthored}, nil
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
				return Call{Name: name, Span: p.spanAt(start), Origin: OriginAuthored}, nil
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
					return Call{Name: name, Args: args, Span: p.spanAt(start), Origin: OriginAuthored}, nil
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
	return Literal{Value: v, Span: p.spanAt(valueStart), Origin: OriginAuthored}, nil
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
			return Literal{Value: v, Span: p.spanAt(start), Origin: OriginAuthored}, nil
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
