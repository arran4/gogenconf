package codegen

import (
	"strconv"
)

func staticDecoderName(section, field string) string {
	return "decode" + section + field
}

// emitStaticDecoder writes field-specialized decoding from the canonical Expr
// representation. It deliberately does not populate structs through
// reflection: every generated field has an explicit conversion function.
func emitStaticDecoder(p func(string, ...any), fn, goType string, ti TypeInfo, rtAlias, path string, sensitive bool) {
	if ti.IsMap {
		elemInfo := ti
		if ti.Element != nil {
			elemInfo = *ti.Element
		} else {
			elemInfo.GoType, elemInfo.IsSlice, elemInfo.IsMap = ti.ElemType, false, false
		}
		emitStaticDecoder(p, fn+"Element", ti.ElemType, elemInfo, rtAlias, path, sensitive)
		p("func %s(ctx context.Context,r *%s.Registry,e %s.Expr)(%s,error){var out %s;switch v:=e.(type){case %s.Map:out=make(%s,len(v.Entries));for _,entry:=range v.Entries{value,err:=%sElement(ctx,r,entry.Value);if err!=nil{return out,err};out[entry.Key]=value};return out,nil;case *%s.Map:if v==nil{return out,decodeValueError(%q,%t,%s.SpanOf(e),fmt.Errorf(\"nil map\"))};return %s(ctx,r,*v);default:return out,decodeValueError(%q,%t,%s.SpanOf(e),fmt.Errorf(\"expected keyed collection, got %%T\",e))}}\n", fn, rtAlias, rtAlias, goType, goType, rtAlias, goType, fn, rtAlias, path, sensitive, rtAlias, fn, path, sensitive, rtAlias)
		return
	}
	if ti.IsSlice {
		elemInfo := ti
		if ti.Element != nil {
			elemInfo = *ti.Element
		} else {
			elemInfo.GoType, elemInfo.IsSlice, elemInfo.IsMap = ti.ElemType, false, false
		}
		emitStaticDecoder(p, fn+"Element", ti.ElemType, elemInfo, rtAlias, path, sensitive)
		p("func %s(ctx context.Context,r *%s.Registry,e %s.Expr)(%s,error){var out %s;switch v:=e.(type){case %s.List:out=make(%s,0,len(v.Items));for _,item:=range v.Items{value,err:=%sElement(ctx,r,item);if err!=nil{return out,err};out=append(out,value)};return out,nil;case *%s.List:if v==nil{return out,decodeValueError(%q,%t,%s.SpanOf(e),fmt.Errorf(\"nil list\"))};return %s(ctx,r,*v);default:raw,err:=%s.ResolveField[string](ctx,r,e,%q,%t);if err!=nil{return out,err};items:=strings.Split(raw,\",\");out=make(%s,0,len(items));for _,item:=range items{value,err:=%sElement(ctx,r,%s.Literal{Value:item,Span:%s.SpanOf(e)});if err!=nil{return out,err};out=append(out,value)};return out,nil}}\n", fn, rtAlias, rtAlias, goType, goType, rtAlias, goType, fn, rtAlias, path, sensitive, rtAlias, fn, rtAlias, path, sensitive, goType, fn, rtAlias, rtAlias)
		return
	}
	emitStaticScalarDecoder(p, fn, goType, ti, rtAlias, path, sensitive)
}

func staticDecoderCall(fn string, ti TypeInfo) string {
	return fn
}

func emitStaticScalarDecoder(p func(string, ...any), fn, goType string, ti TypeInfo, rtAlias, path string, sensitive bool) {
	if ti.IsPtr {
		valueInfo := ti
		valueInfo.GoType, valueInfo.IsPtr = ti.ElemType, false
		emitStaticScalarDecoder(p, fn+"Value", ti.ElemType, valueInfo, rtAlias, path, sensitive)
		p("func %s(ctx context.Context,r *%s.Registry,e %s.Expr)(%s,error){value,err:=%sValue(ctx,r,e);if err!=nil{return nil,err};return &value,nil}\n", fn, rtAlias, rtAlias, goType, fn)
		return
	}
	// Existing generated applications may use a named underlying-string type
	// that only implements lifecycle hooks. Its underlying representation is
	// not available to this generator, so retain the established resolver
	// bridge until Child B supplies an explicit static adapter.
	if ti.Kind == TypeKindCustom {
		p("func %s(ctx context.Context,r *%s.Registry,e %s.Expr)(%s,error){return %s.ResolveField[%s](ctx,r,e,%q,%t)}\n", fn, rtAlias, rtAlias, goType, rtAlias, goType, path, sensitive)
		return
	}
	p("func %s(ctx context.Context,r *%s.Registry,e %s.Expr)(%s,error){var out %s;var raw string;var span %s.Span;switch v:=e.(type){case %s.Literal:raw=v.Value;span=v.Span;case *%s.Literal:if v==nil{return out,decodeValueError(%q,%t,%s.SpanOf(e),fmt.Errorf(\"nil literal\"))};raw=v.Value;span=v.Span;default:value,err:=%s.ResolveField[string](ctx,r,e,%q,%t);if err!=nil{return out,err};raw=value;span=%s.SpanOf(e)};_=span;", fn, rtAlias, rtAlias, goType, goType, rtAlias, rtAlias, rtAlias, path, sensitive, rtAlias, rtAlias, path, sensitive, rtAlias)
	emitStaticScalarAssignment(p, "out", ti, "raw", "span", rtAlias, path, sensitive)
	p("return out,nil}\n")
}

func emitStaticScalarAssignment(p func(string, ...any), dest string, ti TypeInfo, raw, span, rtAlias, path string, sensitive bool) {
	pathLit, sensitiveLit := strconv.Quote(path), strconv.FormatBool(sensitive)
	fail := func(expr string) {
		p("if err!=nil{return %s,decodeValueError(%s,%s,%s,err)};", dest, pathLit, sensitiveLit, span)
		_ = expr
	}
	switch ti.Kind {
	case TypeKindString:
		p("%s=%s;", dest, raw)
	case TypeKindBytes:
		p("%s=[]byte(%s);", dest, raw)
	case TypeKindBool:
		p("parsed,err:=strconv.ParseBool(%s);", raw)
		fail("")
		p("%s=parsed;", dest)
	case TypeKindInt:
		p("parsed,err:=strconv.ParseInt(%s,0,%d);", raw, typeBitSize(ti))
		fail("")
		p("%s=%s(parsed);", dest, ti.GoType)
	case TypeKindUint:
		p("parsed,err:=strconv.ParseUint(%s,0,%d);", raw, typeBitSize(ti))
		fail("")
		p("%s=%s(parsed);", dest, ti.GoType)
	case TypeKindFloat:
		p("parsed,err:=strconv.ParseFloat(%s,%d);", raw, typeBitSize(ti))
		fail("")
		p("%s=%s(parsed);", dest, ti.GoType)
	case TypeKindDuration:
		p("parsed,err:=time.ParseDuration(%s);", raw)
		fail("")
		p("%s=parsed;", dest)
	case TypeKindCustom:
		p("if dec,ok:=any(&%s).(interface{DecodeConfig(string)error});ok{err:=dec.DecodeConfig(%s);if err!=nil{return %s,decodeValueError(%s,%s,%s,err)}}else if dec,ok:=any(&%s).(interface{UnmarshalText([]byte)error});ok{err:=dec.UnmarshalText([]byte(%s));if err!=nil{return %s,decodeValueError(%s,%s,%s,err)}}else{return %s,decodeValueError(%s,%s,%s,fmt.Errorf(\"unsupported custom decoder\"))};", dest, raw, dest, pathLit, sensitiveLit, span, dest, raw, dest, pathLit, sensitiveLit, span, dest, pathLit, sensitiveLit, span)
	}
}
