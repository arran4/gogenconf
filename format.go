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
	var section *Section
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
		if section != nil {
			items = &section.Items
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
		key, tail, _ := strings.Cut(line, " ")
		// Tabs are separators too.
		for i, c := range line {
			if c == ' ' || c == '\t' {
				key = line[:i]
				tail = strings.TrimSpace(line[i:])
				break
			}
		}
		keyCol := lineCol
		switch key {
		case "config":
			if section != nil || d.HasVersion {
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
			if section != nil || d.HasVersion {
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
			if section != nil || tail == "" {
				return fail(keyCol, "expected a named, non-nested section")
			}
			if d.Section(tail) != nil {
				return fail(keyCol, "duplicate section")
			}
			section = &Section{
				Name: tail,
				Span: Span{
					Source: sourceName,
					Line:   lineNo,
					Column: keyCol,
					Offset: lineOffset + keyCol - 1,
				},
				Origin: OriginAuthored,
			}
			d.AddSection(section)
		case "end":
			if section == nil || tail != "" {
				return fail(keyCol, "unexpected end")
			}
			section = nil
		default:
			if !identifier(key) {
				return fail(keyCol, "invalid field name")
			}
			if section != nil {
				if _, _, ok := section.Entry(key); ok {
					return fail(keyCol, "duplicate field")
				}
			} else {
				if _, _, ok := d.Entry(key); ok {
					return fail(keyCol, "duplicate field")
				}
			}
			tailCol := keyCol + len(key)
			if tail != "" {
				tailIdx := strings.Index(rawLine[keyCol+len(key)-1:], tail)
				if tailIdx >= 0 {
					tailCol = keyCol + len(key) + tailIdx
				}
			}
			e, err := ParseExprWithSpan(tail, Span{
				Source: sourceName,
				Line:   lineNo,
				Column: tailCol,
				Offset: lineOffset + tailCol - 1,
			})
			if err != nil {
				return fail(tailCol, "invalid expression: "+err.Error())
			}
			// A legacy value-less entry is retained but is not an override.
			// Explicit empty strings use "".
			if tail == "" {
				e = nil
			}
			*items = append(*items, Entry{
				Key:   key,
				Value: e,
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
	if section != nil {
		return fail(1, "unterminated section")
	}
	if !d.HasVersion || d.Version == 0 {
		for _, c := range legacyComments {
			(*c.items)[c.index] = Comment{Kind: UserComment, Text: c.text}
		}
	}
	return d, nil
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
				fmt.Fprintf(&b, "section %s\n", v.Name)
				write(v.Items, "    ")
				b.WriteString("end\n")
			case Entry:
				if v.Value == nil {
					fmt.Fprintf(&b, "%s%s\n", indent, v.Key)
				} else {
					fmt.Fprintf(&b, "%s%s %s\n", indent, v.Key, FormatExpr(v.Value))
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
