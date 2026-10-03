// Package codegen turns a validated schema into static Go types and binders.
// It has no knowledge of application-specific fields or packages.
package codegen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	m "github.com/arran4/gogenconf"
)

type Options struct {
	// Package is the destination package name.
	Package string
	// LibraryImport is the configuration library path in generated code.
	// Empty uses github.com/arran4/gogenconf.
	LibraryImport string
	// ConfigImport selects the separately generated application model package.
	ConfigImport string
}

func Validate(s m.Schema) error {
	if s.Version < 1 {
		return fmt.Errorf("schema requires a positive version")
	}
	paths, names, typesSeen := map[string]bool{}, map[string]bool{}, map[string]bool{"Config": true}
	for _, sec := range s.Sections {
		if sec.Name == "" || paths[sec.Name] {
			return fmt.Errorf("duplicate or empty section %q", sec.Name)
		}
		paths[sec.Name] = true
		if !exported(sec.GoName) || names[sec.GoName] {
			return fmt.Errorf("invalid or ambiguous section mapping %q", sec.GoName)
		}
		names[sec.GoName] = true
		if !exported(sec.GoType) || typesSeen[sec.GoType] {
			return fmt.Errorf("invalid or ambiguous type %q", sec.GoType)
		}
		typesSeen[sec.GoType] = true
		keys, fields := map[string]bool{}, map[string]bool{}
		if sec.Repeated {
			fields["Name"] = true
		}
		for _, f := range sec.Fields {
			for _, e := range f.Examples {
				if e == nil {
					return fmt.Errorf("nil example for %s.%s", sec.Name, f.Key)
				}
				if _, err := expression(e); err != nil {
					return fmt.Errorf("invalid example for %s.%s: %w", sec.Name, f.Key, err)
				}
				if _, err := m.ParseExpr(m.FormatExpr(e)); err != nil {
					return fmt.Errorf("invalid example syntax for %s.%s", sec.Name, f.Key)
				}
			}
			if f.InputOnly {
				if f.Key == "" || keys[f.Key] || f.RuntimeOnly || f.Policy != m.Eager || f.GoName != "" || f.GoType != "" {
					return fmt.Errorf("invalid input-only field %s.%s", sec.Name, f.Key)
				}
				keys[f.Key] = true
				continue
			}
			if !exported(f.GoName) || fields[f.GoName] {
				return fmt.Errorf("invalid or duplicate field mapping %s.%s", sec.GoName, f.GoName)
			}
			fields[f.GoName] = true
			if !f.RuntimeOnly {
				if f.Key == "" || keys[f.Key] {
					return fmt.Errorf("duplicate or empty config path %s.%s", sec.Name, f.Key)
				}
				keys[f.Key] = true
			}
			tv, err := types.Eval(token.NewFileSet(), nil, token.NoPos, f.GoType)
			if err != nil || !tv.IsType() {
				return fmt.Errorf("invalid Go type for %s.%s", sec.Name, f.Key)
			}
			switch f.GoType {
			case "string", "[]byte", "[]string":
			case "bool":
				if !f.RuntimeOnly {
					return fmt.Errorf("bool resolution requires an explicit mapping")
				}
			default:
				return fmt.Errorf("unsupported type mapping %s", f.GoType)
			}
			if f.Policy != m.Eager && f.Policy != m.ProviderBacked {
				return fmt.Errorf("unsupported resolution policy")
			}
			if f.RuntimeOnly && (f.Default != nil || f.Environment != "" || f.Policy != m.Eager || f.Key != "") {
				return fmt.Errorf("runtime-only field has persisted policy")
			}
			if _, err := expression(f.Default); err != nil {
				return fmt.Errorf("%s.%s: %w", sec.Name, f.Key, err)
			}
		}
	}
	return nil
}
func exported(s string) bool { return token.IsIdentifier(s) && ast.IsExported(s) }

func expression(e m.Expr) (string, error) {
	switch v := e.(type) {
	case nil:
		return "nil", nil
	case m.Literal:
		return "gogenconf.Literal{Value:" + strconv.Quote(v.Value) + "}", nil
	case *m.Literal:
		return expression(*v)
	case m.Call:
		args := []string{}
		for _, a := range v.Args {
			r, err := expression(a)
			if err != nil {
				return "", err
			}
			args = append(args, r)
		}
		return "gogenconf.Call{Name:" + strconv.Quote(v.Name) + ",Args:[]gogenconf.Expr{" + strings.Join(args, ",") + "}}", nil
	case *m.Call:
		return expression(*v)
	default:
		return "", fmt.Errorf("unsupported default expression")
	}
}

// Generate validates before producing any output. Go names/types are checked
// at generation time; the Go compiler checks the emitted typed resolver calls.
func Generate(s m.Schema, o Options) ([]byte, error) {
	return generate(s, o, true, true)
}

// GenerateModel emits only concrete application-owned types.
func GenerateModel(s m.Schema, o Options) ([]byte, error) {
	return generate(s, o, true, false)
}

// GenerateBinder emits one-way typed resolution into a separate model package.
func GenerateBinder(s m.Schema, o Options) ([]byte, error) {
	if o.ConfigImport == "" {
		return nil, fmt.Errorf("runtime model import is required")
	}
	return generate(s, o, false, true)
}

func generate(s m.Schema, o Options, model, binder bool) ([]byte, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}
	if !token.IsIdentifier(o.Package) || o.Package == "_" {
		return nil, fmt.Errorf("invalid package")
	}
	if o.LibraryImport == "" {
		o.LibraryImport = "github.com/arran4/gogenconf"
	}
	var b bytes.Buffer
	p := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	p("// Code generated by gogenconf. DO NOT EDIT.\n\npackage %s\n", o.Package)
	qual := ""
	if binder {
		p("import(\"context\";\"fmt\";\"strings\"; gogenconf %q)\n", o.LibraryImport)
		if !model {
			p("import runtimeconfig %q\n", o.ConfigImport)
			qual = "runtimeconfig."
		}
		p("var _ = strings.Join\nvar _ = fmt.Errorf\n")
	} else {
		usesProvider := false
		for _, sec := range s.Sections {
			for _, f := range sec.Fields {
				usesProvider = usesProvider || !f.InputOnly && f.Policy == m.ProviderBacked
			}
		}
		if usesProvider {
			p("import gogenconf %q\n", o.LibraryImport)
		}
	}
	if model {
		p("type Config struct {\n")
		for _, sec := range s.Sections {
			prefix := ""
			if sec.Repeated {
				prefix = "[]"
			}
			p("%s %s%s\n", sec.GoName, prefix, sec.GoType)
		}
		p("}\n")
		for _, sec := range s.Sections {
			p("type %s struct {\n", sec.GoType)
			if sec.Repeated {
				p("Name string\n")
			}
			for _, f := range sec.Fields {
				if f.InputOnly {
					continue
				}
				typ := f.GoType
				if f.Policy == m.ProviderBacked {
					typ = "gogenconf.Provider[" + typ + "]"
				}
				p("%s %s\n", f.GoName, typ)
			}
			p("}\n")
		}
	}
	if !binder {
		return format.Source(b.Bytes())
	}
	p("// Defaults returns concrete static defaults and optional legacy environment overrides.\nfunc Defaults(lookup gogenconf.LookupEnv) %sConfig { var c %sConfig\n", qual, qual)
	for _, sec := range s.Sections {
		if sec.Repeated {
			continue
		}
		for _, f := range sec.Fields {
			if f.InputOnly || f.RuntimeOnly || f.Policy == m.ProviderBacked {
				continue
			}
			dest := "c." + sec.GoName + "." + f.GoName
			if v, ok := f.Default.(m.Literal); ok {
				switch f.GoType {
				case "string":
					p("%s=%q\n", dest, v.Value)
				case "[]byte":
					p("%s=[]byte(%q)\n", dest, v.Value)
				case "[]string":
					p("%s=strings.FieldsFunc(%q,func(r rune)bool{return r==','})\n", dest, v.Value)
				}
			}
			if f.Environment != "" {
				p("if lookup!=nil{if v,ok:=lookup(%q);ok", f.Environment)
				if f.EnvironmentNonEmpty {
					p("&&v!=\"\"")
				}
				p("{")
				switch f.GoType {
				case "string":
					p("%s=v", dest)
				case "[]byte":
					p("%s=[]byte(v)", dest)
				case "[]string":
					p("%s=strings.Split(v,\",\")", dest)
				}
				p("}}\n")
			}
		}
	}
	p("return c}\n")
	p("// Resolve constructs ordinary Go fields; provider policy alone defers resolution.\nfunc Resolve(ctx context.Context,d *gogenconf.Document,r *gogenconf.Registry,lookup gogenconf.LookupEnv)(%sConfig,error){\nvar c %sConfig\nif d==nil{return c,fmt.Errorf(\"nil document\")}\nif d.Version>%d{return c,fmt.Errorf(\"unsupported future configuration version\")}\n", qual, qual, s.Version)
	for _, sec := range s.Sections {
		section := strconv.Quote(sec.Name)
		dest := "c." + sec.GoName
		if sec.Repeated {
			p("for _,section:=range d.Sections{if !strings.HasPrefix(section.Name,%q){continue};var v %s%s;v.Name=strings.TrimPrefix(section.Name,%q)\n", sec.Name+" ", qual, sec.GoType, sec.Name+" ")
			section = "section.Name"
			dest = "v"
		}
		for _, f := range sec.Fields {
			if f.InputOnly || f.RuntimeOnly {
				continue
			}
			ex, _ := expression(f.Default)
			p("{e:=gogenconf.BindingValue(d,%s,%q,%s,%q,%t,lookup)\n", section, f.Key, ex, f.Environment, f.EnvironmentNonEmpty)
			if f.Required {
				p("if e==nil{return %sConfig{},fmt.Errorf(%q)}\n", qual, "missing required field "+sec.Name+"."+f.Key)
			}
			p("if e!=nil{\n")
			if f.Policy == m.ProviderBacked {
				p("%s.%s=gogenconf.FieldProvider[%s](r,e,%q,%t)\n", dest, f.GoName, f.GoType, sec.Name+"."+f.Key, f.Sensitive)
			} else {
				p("value,err:=gogenconf.ResolveField[%s](ctx,r,e,%q,%t);if err!=nil{return %sConfig{},err};%s.%s=value\n", f.GoType, sec.Name+"."+f.Key, f.Sensitive, qual, dest, f.GoName)
			}
			p("}}\n")
		}
		if sec.Repeated {
			p("c.%s=append(c.%s,v)}\n", sec.GoName, sec.GoName)
		}
	}
	p("return c,nil}\n")
	return format.Source(b.Bytes())
}
