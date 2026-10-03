package configmodel

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse reads the native language. Section/field names are schema-independent.
// Versions are root metadata; unversioned input is explicitly legacy version 0.
// Duplicate fields/sections and malformed delimiters are rejected, not guessed.
func Parse(r io.Reader) (*Document, error) {
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
	fail := func(message string) (*Document, error) {
		return nil, fmt.Errorf("configuration line %d: %s", lineNo, message)
	}
	for scan.Scan() {
		lineNo++
		line := strings.TrimSpace(scan.Text())
		items := &d.Items
		if section != nil {
			items = &section.Items
		}
		if line == "" {
			*items = append(*items, Blank{})
			continue
		}
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
		switch key {
		case "config_version":
			if section != nil || d.HasVersion {
				return fail("config_version must occur once at root")
			}
			v, err := strconv.Atoi(tail)
			if err != nil || v < 0 {
				return fail("invalid config_version")
			}
			d.Version = v
			d.HasVersion = true
			d.Items = append(d.Items, Version{})
		case "section":
			if section != nil || tail == "" {
				return fail("expected a named, non-nested section")
			}
			if d.Section(tail) != nil {
				return fail("duplicate section")
			}
			section = &Section{Name: tail}
			d.AddSection(section)
		case "end":
			if section == nil || tail != "" {
				return fail("unexpected end")
			}
			section = nil
		default:
			if !identifier(key) {
				return fail("invalid field name")
			}
			if section != nil {
				if _, _, ok := section.Entry(key); ok {
					return fail("duplicate field")
				}
			}
			e, err := ParseExpr(tail)
			if err != nil {
				return fail("invalid expression: " + err.Error())
			}
			// A legacy value-less entry is retained but is not an override.
			// Explicit empty strings use "".
			if tail == "" {
				e = nil
			}
			*items = append(*items, Entry{Key: key, Value: e})
		}
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if section != nil {
		return fail("unterminated section")
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

// Format never resolves values and retains the position of every root item,
// including comments between sections and trailing comments.
func Format(d *Document) string {
	var b strings.Builder
	var write func([]Item, string)
	write = func(items []Item, indent string) {
		for _, item := range items {
			switch v := item.(type) {
			case Version:
				if d.HasVersion {
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
		case Version:
			version = true
		case *Section:
			sections = true
		}
	}
	if d.HasVersion && !version {
		fmt.Fprintf(&b, "config_version %d\n", d.Version)
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
