package gogenconf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse reads the native language. Section/field names are schema-independent.
// Versions are root metadata; unversioned input is explicitly legacy version 0.
// Duplicate fields/sections and malformed delimiters are rejected, not guessed.
func Parse(r io.Reader) (*Document, error) {
	return ParseNamed(r, "")
}

// ParseNamed reads the native language and attaches source name to node spans.
func ParseNamed(r io.Reader, sourceName string) (*Document, error) {
	d := &Document{}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 4096), 1024*1024)
	var sectionStack []*Section
	currentSection := func() *Section {
		if len(sectionStack) == 0 {
			return nil
		}
		return sectionStack[len(sectionStack)-1]
	}
	type blockFrame struct {
		isBrace bool
		count   int
		span    Span
	}
	var blockStack []blockFrame
	// Version metadata may appear after comments. Keep ambiguous spellings
	// privately until the whole document's ownership convention is known.
	var legacyComments []struct {
		items *[]Item
		index int
		text  string
	}
	lineNo := 0
	lineOffset := 0
	fail := func(col int, message string) (*Document, error) {
		return nil, &ConfigError{
			Stage: StageParse,
			Source: Span{
				Source: sourceName,
				Line:   lineNo,
				Column: col,
				Offset: lineOffset + col - 1,
			},
			Err: errors.New(message),
		}
	}
	for scan.Scan() {
		lineNo++
		rawLine := scan.Text()
		line := strings.TrimSpace(rawLine)
		items := &d.Items
		if sec := currentSection(); sec != nil {
			items = &sec.Items
		}
		if line == "" {
			*items = append(*items, Blank{})
			lineOffset += len(rawLine) + 1
			continue
		}
		lead := strings.Index(rawLine, line)
		lineCol := lead + 1
		if strings.HasPrefix(line, "#") {
			kind := UserComment
			prefix := "#"
			if strings.HasPrefix(line, "##") {
				kind = DocumentationComment
				prefix = "##"
				legacyComments = append(legacyComments, struct {
					items *[]Item
					index int
					text  string
				}{items, len(*items), strings.TrimPrefix(line[1:], " ")})
			}
			*items = append(*items, Comment{Kind: kind, Text: strings.TrimPrefix(strings.TrimPrefix(line, prefix), " ")})
			lineOffset += len(rawLine) + 1
			continue
		}

		// 1. Closing brace '}'
		if strings.HasPrefix(line, "}") {
			rem := strings.TrimSpace(line[1:])
			if rem != "" && !strings.HasPrefix(rem, "#") {
				return fail(lineCol+1, "unexpected token after '}'")
			}
			if len(blockStack) == 0 {
				return fail(lineCol, "unexpected '}'")
			}
			top := blockStack[len(blockStack)-1]
			if !top.isBrace {
				return fail(lineCol, "unexpected '}', expected 'end'")
			}
			blockStack = blockStack[:len(blockStack)-1]
			if top.count > len(sectionStack) {
				return fail(lineCol, "internal parser error: invalid section stack depth")
			}
			sectionStack = sectionStack[:len(sectionStack)-top.count]
			lineOffset += len(rawLine) + 1
			continue
		}

		// 2. Opening brace '{' (named objects, anonymous objects, and compact paths)
		beforeBrace, hasBrace, emptyBrace, braceColOffset, err := findOpeningBrace(line)
		if err != nil {
			return fail(lineCol, err.Error())
		}
		if hasBrace {
			prefixMut := MutationNone
			if pRem, pKind := splitPrefixMutation(beforeBrace); pKind != MutationNone {
				prefixMut = pKind
				beforeBrace = pRem
			}
			comps, err := parsePathComponents(beforeBrace, lineCol)
			if err != nil {
				return fail(lineCol, err.Error())
			}
			if len(comps) == 0 {
				// Anonymous object/block '{'
				comps = []pathComponent{{name: "", mutation: prefixMut, col: lineCol + braceColOffset}}
			} else {
				// A compact declaration path names the addressed declaration as a
				// whole. In `project[~] lxa {}`, the modifier therefore applies to
				// the leaf (`lxa`), not the intermediate `project` namespace. This
				// is essential for [=] and [-], which must not replace or remove
				// sibling named declarations.
				leaf := len(comps) - 1
				if prefixMut != MutationNone && comps[leaf].mutation == MutationNone {
					comps[leaf].mutation = prefixMut
				}
				if len(comps) > 1 && comps[0].mutation != MutationNone {
					if comps[leaf].mutation != MutationNone {
						return fail(comps[0].col, "multiple mutation modifiers in declaration path")
					}
					comps[leaf].mutation = comps[0].mutation
					comps[0].mutation = MutationNone
				}
			}
			span := Span{
				Source: sourceName,
				Line:   lineNo,
				Column: lineCol,
				Offset: lineOffset + lineCol - 1,
			}
			blockStack = append(blockStack, blockFrame{
				isBrace: true,
				count:   len(comps),
				span:    span,
			})
			for k, comp := range comps {
				curr := currentSection()
				var nextSec *Section
				// Re-use intermediate section if already present in current container
				if k < len(comps)-1 {
					if curr != nil {
						nextSec = curr.Section(comp.name)
					} else {
						nextSec = d.Section(comp.name)
					}
				}
				if nextSec == nil {
					if (!d.HasVersion || d.Version == 0) && comp.name != "" {
						if curr != nil && curr.Section(comp.name) != nil {
							return fail(comp.col, fmt.Sprintf("duplicate section %q", comp.name))
						}
						if curr == nil && d.Section(comp.name) != nil {
							return fail(comp.col, fmt.Sprintf("duplicate section %q", comp.name))
						}
					}
					nextSec = &Section{
						Name:     comp.name,
						Mutation: comp.mutation,
						Brace:    true,
						Span: Span{
							Source: sourceName,
							Line:   lineNo,
							Column: comp.col,
							Offset: lineOffset + comp.col - 1,
						},
						Origin: OriginAuthored,
					}
					if curr != nil {
						curr.AddSection(nextSec)
					} else {
						d.AddSection(nextSec)
					}
				} else if comp.mutation != MutationNone {
					nextSec.Mutation = comp.mutation
				}
				sectionStack = append(sectionStack, nextSec)
			}
			if emptyBrace {
				blockStack = blockStack[:len(blockStack)-1]
				sectionStack = sectionStack[:len(sectionStack)-len(comps)]
			}
			lineOffset += len(rawLine) + 1
			continue
		}

		key, tail, _ := strings.Cut(line, " ")
		// Tabs are separators too.
		for i, c := range line {
			if c == ' ' || c == '\t' {
				key = line[:i]
				tail = strings.TrimSpace(line[i:])
				break
			}
		}
		entryMutation := MutationNone
		if strings.HasPrefix(line, "[+]") || strings.HasPrefix(line, "[=]") || strings.HasPrefix(line, "[~]") || strings.HasPrefix(line, "[-]") {
			// Accepted alternative prefix syntax: `[=] target ./cmd/lxa`
			switch line[:3] {
			case "[+]":
				entryMutation = MutationAdd
			case "[=]":
				entryMutation = MutationReplace
			case "[~]":
				entryMutation = MutationMutate
			case "[-]":
				entryMutation = MutationDelete
			}
			rest := strings.TrimSpace(line[3:])
			key = rest
			tail = ""
			for i, c := range rest {
				if c == ' ' || c == '\t' {
					key = rest[:i]
					tail = strings.TrimSpace(rest[i:])
					break
				}
			}
		} else {
			// Canonical declaration-attached syntax: `target[=] ./cmd/lxa` or `deprecated_field[-]`
			base, mut, hasMut := splitMutation(key)
			if hasMut {
				key = base
				entryMutation = mut
			} else if strings.HasPrefix(tail, "[+]") || strings.HasPrefix(tail, "[=]") || strings.HasPrefix(tail, "[~]") || strings.HasPrefix(tail, "[-]") {
				// Accepted alternative infix operator: `target [=] ./cmd/lxa`
				switch tail[:3] {
				case "[+]":
					entryMutation = MutationAdd
				case "[=]":
					entryMutation = MutationReplace
				case "[~]":
					entryMutation = MutationMutate
				case "[-]":
					entryMutation = MutationDelete
				}
				tail = strings.TrimSpace(tail[3:])
			}
		}
		keyCol := lineCol
		if entryMutation != MutationNone && (strings.HasPrefix(line, "[+]") || strings.HasPrefix(line, "[=]") || strings.HasPrefix(line, "[~]") || strings.HasPrefix(line, "[-]")) {
			idx := strings.Index(line[3:], key)
			if idx >= 0 {
				keyCol = lineCol + 3 + idx
			}
		}
		switch key {
		case "config":
			if currentSection() != nil || d.HasVersion {
				return fail(keyCol, "config header must occur once at root")
			}
			fields := strings.Fields(tail)
			if len(fields) != 4 || fields[0] != "for" || fields[2] != "version" {
				return fail(keyCol, "invalid config header: expected 'config for <identity> version <number>'")
			}
			identity := fields[1]
			if !validIdentity(identity) {
				return fail(keyCol, fmt.Sprintf("invalid configuration identity %q", identity))
			}
			v, err := strconv.Atoi(fields[3])
			if err != nil || v < 0 {
				return fail(keyCol, "invalid config version in header")
			}
			d.Identity = identity
			d.Version = v
			d.HasVersion = true
			headerSpan := Span{
				Source: sourceName,
				Line:   lineNo,
				Column: keyCol,
				Offset: lineOffset + keyCol - 1,
			}
			d.HeaderSpan = headerSpan
			d.Items = append(d.Items, Header{
				Identity: identity,
				Version:  v,
				Span:     headerSpan,
			})
		case "config_version":
			if currentSection() != nil || d.HasVersion {
				return fail(keyCol, "config_version must occur once at root")
			}
			v, err := strconv.Atoi(tail)
			if err != nil || v < 0 {
				return fail(keyCol, "invalid config_version")
			}
			d.Version = v
			d.HasVersion = true
			headerSpan := Span{
				Source: sourceName,
				Line:   lineNo,
				Column: keyCol,
				Offset: lineOffset + keyCol - 1,
			}
			d.HeaderSpan = headerSpan
			d.Items = append(d.Items, Version{})
		case "section":
			if tail == "" {
				return fail(keyCol, "expected a named section")
			}
			secName := tail
			secMut := entryMutation
			if secMut == MutationNone {
				base, m, hasM := splitMutation(secName)
				if hasM {
					secName = base
					secMut = m
				}
			}
			if !d.HasVersion || d.Version == 0 {
				curr := currentSection()
				if curr != nil {
					if curr.Section(secName) != nil {
						return fail(keyCol, fmt.Sprintf("duplicate section %q", secName))
					}
				} else {
					if d.Section(secName) != nil {
						return fail(keyCol, fmt.Sprintf("duplicate section %q", secName))
					}
				}
			}
			newSec := &Section{
				Name:     secName,
				Mutation: secMut,
				Brace:    false,
				Span: Span{
					Source: sourceName,
					Line:   lineNo,
					Column: keyCol,
					Offset: lineOffset + keyCol - 1,
				},
				Origin: OriginAuthored,
			}
			curr := currentSection()
			if curr != nil {
				curr.AddSection(newSec)
			} else {
				d.AddSection(newSec)
			}
			sectionStack = append(sectionStack, newSec)
			blockStack = append(blockStack, blockFrame{
				isBrace: false,
				count:   1,
				span: Span{
					Source: sourceName,
					Line:   lineNo,
					Column: keyCol,
					Offset: lineOffset + keyCol - 1,
				},
			})
		case "end":
			if tail != "" {
				return fail(keyCol, "unexpected tokens after end")
			}
			if len(blockStack) == 0 {
				return fail(keyCol, "unexpected end")
			}
			top := blockStack[len(blockStack)-1]
			if top.isBrace {
				return fail(keyCol, "unexpected 'end', expected '}'")
			}
			blockStack = blockStack[:len(blockStack)-1]
			sectionStack = sectionStack[:len(sectionStack)-1]
		default:
			if !identifier(key) {
				return fail(keyCol, "invalid field name")
			}
			if !d.HasVersion || d.Version == 0 {
				if sec := currentSection(); sec != nil {
					if _, _, ok := sec.Entry(key); ok {
						return fail(keyCol, "duplicate field")
					}
				} else {
					if _, _, ok := d.Entry(key); ok {
						return fail(keyCol, "duplicate field")
					}
				}
			}
			// Note: Duplicate authored entries are preserved in unresolved AST per contract.
			tailCol := keyCol + len(key)
			if tail != "" {
				tailIdx := strings.Index(rawLine[keyCol+len(key)-1:], tail)
				if tailIdx >= 0 {
					tailCol = keyCol + len(key) + tailIdx
				}
			}
			var e Expr
			if tail != "" {
				var err error
				e, err = ParseExprWithSpan(tail, Span{
					Source: sourceName,
					Line:   lineNo,
					Column: tailCol,
					Offset: lineOffset + tailCol - 1,
				})
				if err != nil {
					return fail(tailCol, "invalid expression: "+err.Error())
				}
			}
			// A legacy value-less entry is retained but is not an override.
			// Explicit empty strings use "".
			*items = append(*items, Entry{
				Key:      key,
				Mutation: entryMutation,
				Value:    e,
				Span: Span{
					Source: sourceName,
					Line:   lineNo,
					Column: keyCol,
					Offset: lineOffset + keyCol - 1,
				},
				Origin: OriginAuthored,
			})
		}
		lineOffset += len(rawLine) + 1
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if len(blockStack) > 0 {
		top := blockStack[len(blockStack)-1]
		if top.isBrace {
			return fail(top.span.Column, "unterminated block, missing '}'")
		}
		return fail(top.span.Column, "unterminated section, missing 'end'")
	}
	if !d.HasVersion || d.Version == 0 {
		for _, c := range legacyComments {
			(*c.items)[c.index] = Comment{Kind: UserComment, Text: c.text}
		}
	}
	return d, nil
}

func findOpeningBrace(line string) (string, bool, bool, int, error) {
	n := len(line)
	inQuote := false
	escaped := false
	for i := 0; i < n; i++ {
		c := line[i]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inQuote = false
			}
			continue
		}
		if c == '"' {
			inQuote = true
			continue
		}
		if c == '#' {
			// Rest of line is comment
			break
		}
		if c == '{' {
			rem := strings.TrimSpace(line[i+1:])
			if rem == "" || strings.HasPrefix(rem, "#") {
				return strings.TrimSpace(line[:i]), true, false, i, nil
			}
			if rem == "}" {
				return strings.TrimSpace(line[:i]), true, true, i, nil
			}
		}
	}
	if inQuote {
		return "", false, false, 0, errors.New("unterminated quote")
	}
	return "", false, false, 0, nil
}

type pathComponent struct {
	name     string
	mutation MutationKind
	col      int
	quoted   bool
}

func splitMutation(s string) (string, MutationKind, bool) {
	for _, op := range []struct {
		str  string
		kind MutationKind
	}{
		{"[+]", MutationAdd},
		{"[=]", MutationReplace},
		{"[~]", MutationMutate},
		{"[-]", MutationDelete},
	} {
		if strings.HasSuffix(s, op.str) {
			return s[:len(s)-len(op.str)], op.kind, true
		}
	}
	return s, MutationNone, false
}

func splitPrefixMutation(s string) (string, MutationKind) {
	trimmed := strings.TrimLeft(s, " \t")
	for _, op := range []struct {
		str  string
		kind MutationKind
	}{
		{"[+]", MutationAdd},
		{"[=]", MutationReplace},
		{"[~]", MutationMutate},
		{"[-]", MutationDelete},
	} {
		if strings.HasPrefix(trimmed, op.str) {
			rem := strings.TrimLeft(trimmed[len(op.str):], " \t")
			return rem, op.kind
		}
	}
	return s, MutationNone
}

func parsePathComponents(s string, startCol int) ([]pathComponent, error) {
	var comps []pathComponent
	i := 0
	n := len(s)
	for i < n {
		for i < n && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= n {
			break
		}
		col := startCol + i
		if s[i] == '"' {
			start := i
			i++
			escaped := false
			closed := false
			for i < n {
				if escaped {
					escaped = false
					i++
					continue
				}
				if s[i] == '\\' {
					escaped = true
					i++
					continue
				}
				if s[i] == '"' {
					closed = true
					i++
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted path component at column %d", col)
			}
			raw := s[start:i]
			unquoted, err := strconv.Unquote(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid quoted path component at column %d: %w", col, err)
			}
			var mut MutationKind
			if i+3 <= n {
				if _, m, ok := splitMutation(s[i : i+3]); ok {
					mut = m
					i += 3
				}
			}
			comps = append(comps, pathComponent{name: unquoted, mutation: mut, col: col, quoted: true})
		} else {
			start := i
			for i < n && s[i] != ' ' && s[i] != '\t' && s[i] != '{' {
				i++
			}
			raw := s[start:i]
			base, mut, hasMut := splitMutation(raw)
			if hasMut {
				if base == "" {
					return nil, fmt.Errorf("invalid path component %q at column %d", raw, col)
				}
				if !validPathComponent(base) {
					return nil, fmt.Errorf("invalid path component %q at column %d", base, col)
				}
				comps = append(comps, pathComponent{name: base, mutation: mut, col: col, quoted: false})
			} else {
				if !validPathComponent(raw) {
					return nil, fmt.Errorf("invalid path component %q at column %d", raw, col)
				}
				comps = append(comps, pathComponent{name: raw, mutation: MutationNone, col: col, quoted: false})
			}
		}
	}
	return comps, nil
}

func validPathComponent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range []byte(s) {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.' || c == '/')) {
			return false
		}
	}
	return true
}

func needsQuote(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range []byte(s) {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.' || c == '/')) {
			return true
		}
	}
	return false
}

func identifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range []byte(s) {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.')) {
			return false
		}
	}
	return true
}

func validIdentity(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range []byte(s) {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.' || c == '/')) {
			return false
		}
	}
	return true
}

// Format never resolves values and retains the position of every root item,
// including comments between sections and trailing comments.
func Format(d *Document) string {
	var b strings.Builder
	var write func([]Item, string)
	write = func(items []Item, indent string) {
		for _, item := range items {
			switch v := item.(type) {
			case Header:
				if d.Identity != "" {
					fmt.Fprintf(&b, "config for %s version %d\n", d.Identity, d.Version)
				} else if d.HasVersion {
					fmt.Fprintf(&b, "config_version %d\n", d.Version)
				}
			case Version:
				if d.Identity != "" {
					fmt.Fprintf(&b, "config for %s version %d\n", d.Identity, d.Version)
				} else if d.HasVersion {
					fmt.Fprintf(&b, "config_version %d\n", d.Version)
				}
			case *Section:
				mutStr := ""
				if v.Mutation != MutationNone {
					mutStr = v.Mutation.String()
				}
				if v.Brace {
					if v.Name == "" {
						if mutStr != "" {
							fmt.Fprintf(&b, "%s%s {\n", indent, mutStr)
						} else {
							fmt.Fprintf(&b, "%s{\n", indent)
						}
					} else if needsQuote(v.Name) {
						fmt.Fprintf(&b, "%s%s%s {\n", indent, strconv.Quote(v.Name), mutStr)
					} else {
						fmt.Fprintf(&b, "%s%s%s {\n", indent, v.Name, mutStr)
					}
					subSections := false
					for _, i := range v.Items {
						if _, ok := i.(*Section); ok {
							subSections = true
							break
						}
					}
					write(v.Items, indent+"    ")
					if !subSections && len(v.Sections) > 0 {
						for _, sub := range v.Sections {
							write([]Item{sub}, indent+"    ")
						}
					}
					fmt.Fprintf(&b, "%s}\n", indent)
				} else {
					if mutStr != "" {
						fmt.Fprintf(&b, "%ssection%s %s\n", indent, mutStr, v.Name)
					} else {
						fmt.Fprintf(&b, "%ssection %s\n", indent, v.Name)
					}
					subSections := false
					for _, i := range v.Items {
						if _, ok := i.(*Section); ok {
							subSections = true
							break
						}
					}
					write(v.Items, indent+"    ")
					if !subSections && len(v.Sections) > 0 {
						for _, sub := range v.Sections {
							write([]Item{sub}, indent+"    ")
						}
					}
					fmt.Fprintf(&b, "%send\n", indent)
				}
			case Entry:
				mutStr := ""
				if v.Mutation != MutationNone {
					mutStr = v.Mutation.String()
				}
				if v.Value == nil {
					fmt.Fprintf(&b, "%s%s%s\n", indent, v.Key, mutStr)
				} else {
					fmt.Fprintf(&b, "%s%s%s %s\n", indent, v.Key, mutStr, FormatExpr(v.Value))
				}
			case Comment:
				marker := "#"
				if v.Kind == DocumentationComment {
					marker = "##"
				}
				fmt.Fprintf(&b, "%s%s", indent, marker)
				if v.Text != "" {
					if v.Kind == UserComment && (!d.HasVersion || d.Version == 0) && strings.HasPrefix(v.Text, "#") {
						// Retain legacy heading spelling. In v1 the separating
						// space is mandatory to retain user ownership on reload.
						b.WriteString(v.Text)
					} else {
						b.WriteString(" " + v.Text)
					}
				}
				b.WriteByte('\n')
			case Blank:
				b.WriteByte('\n')
			case Raw:
				fmt.Fprintf(&b, "%s%s\n", indent, v.Text)
			}
		}
	}
	version, sections := false, false
	for _, i := range d.Items {
		switch i.(type) {
		case Header, Version:
			version = true
		case *Section:
			sections = true
		}
	}
	if d.HasVersion && !version {
		if d.Identity != "" {
			fmt.Fprintf(&b, "config for %s version %d\n", d.Identity, d.Version)
		} else {
			fmt.Fprintf(&b, "config_version %d\n", d.Version)
		}
	}
	write(d.Items, "")
	if !sections {
		for i, s := range d.Sections {
			if i > 0 {
				b.WriteByte('\n')
			}
			write([]Item{s}, "")
		}
	}
	return b.String()
}
