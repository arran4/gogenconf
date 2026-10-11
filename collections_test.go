package gogenconf_test

import (
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
)

func TestSchemaCollectionTypeModel(t *testing.T) {
	tests := []struct {
		goType       string
		wantElem     string
		wantKind     gogenconf.CollectionKind
		wantResolved string
	}{
		// Scalar types
		{"string", "string", gogenconf.CollectionNone, "string"},
		{"int", "int", gogenconf.CollectionNone, "int"},
		{"time.Duration", "time.Duration", gogenconf.CollectionNone, "time.Duration"},
		{"[]byte", "[]byte", gogenconf.CollectionNone, "[]byte"},

		// Ordered collections ([]T)
		{"[]string", "string", gogenconf.CollectionOrdered, "[]string"},
		{"[]int", "int", gogenconf.CollectionOrdered, "[]int"},
		{"[]time.Duration", "time.Duration", gogenconf.CollectionOrdered, "[]time.Duration"},
		{"[]WorkerConfig", "WorkerConfig", gogenconf.CollectionOrdered, "[]WorkerConfig"},

		// Keyed collections ([$]T and map[string]T)
		{"[$]string", "string", gogenconf.CollectionKeyed, "map[string]string"},
		{"[$]int", "int", gogenconf.CollectionKeyed, "map[string]int"},
		{"[$]WorkerConfig", "WorkerConfig", gogenconf.CollectionKeyed, "map[string]WorkerConfig"},
		{"map[string]string", "string", gogenconf.CollectionKeyed, "map[string]string"},
		{"map[string]int", "int", gogenconf.CollectionKeyed, "map[string]int"},
		{"map[string]WorkerConfig", "WorkerConfig", gogenconf.CollectionKeyed, "map[string]WorkerConfig"},
	}

	for _, tc := range tests {
		t.Run(tc.goType, func(t *testing.T) {
			elem, kind := gogenconf.ParseType(tc.goType)
			if elem != tc.wantElem {
				t.Errorf("ParseType(%q) elem = %q; want %q", tc.goType, elem, tc.wantElem)
			}
			if kind != tc.wantKind {
				t.Errorf("ParseType(%q) kind = %v; want %v", tc.goType, kind, tc.wantKind)
			}

			field := gogenconf.Field{GoType: tc.goType}
			if field.IsCollection() != (tc.wantKind != gogenconf.CollectionNone) {
				t.Errorf("field.IsCollection() = %v; want %v", field.IsCollection(), tc.wantKind != gogenconf.CollectionNone)
			}
			if field.CollectionKind() != tc.wantKind {
				t.Errorf("field.CollectionKind() = %v; want %v", field.CollectionKind(), tc.wantKind)
			}
			if field.ElementType() != tc.wantElem {
				t.Errorf("field.ElementType() = %q; want %q", field.ElementType(), tc.wantElem)
			}
			if field.ResolvedGoType() != tc.wantResolved {
				t.Errorf("field.ResolvedGoType() = %q; want %q", field.ResolvedGoType(), tc.wantResolved)
			}
		})
	}

	// Canonical notation
	if got := gogenconf.CanonicalType("string", gogenconf.CollectionOrdered); got != "[]string" {
		t.Errorf("CanonicalType = %q; want []string", got)
	}
	if got := gogenconf.CanonicalType("WorkerConfig", gogenconf.CollectionKeyed); got != "[$]WorkerConfig" {
		t.Errorf("CanonicalType = %q; want [$]WorkerConfig", got)
	}
}

func TestListExprParseAndFormat(t *testing.T) {
	input := `[ "apple", "banana", "cherry" ]`
	e, err := gogenconf.ParseExpr(input)
	if err != nil {
		t.Fatalf("ParseExpr(%q) failed: %v", input, err)
	}

	list, ok := e.(gogenconf.List)
	if !ok {
		t.Fatalf("expected List, got %T", e)
	}

	if len(list.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list.Items))
	}

	want := []string{"apple", "banana", "cherry"}
	for i, w := range want {
		lit, ok := list.Items[i].(gogenconf.Literal)
		if !ok || lit.Value != w {
			t.Errorf("item %d = %v; want Literal{%q}", i, list.Items[i], w)
		}
	}

	formatted := gogenconf.FormatExpr(list)
	e2, err := gogenconf.ParseExpr(formatted)
	if err != nil {
		t.Fatalf("re-parsing formatted list failed: %v", err)
	}
	list2, ok := e2.(gogenconf.List)
	if !ok || len(list2.Items) != 3 {
		t.Fatalf("unexpected re-parsed list: %v", e2)
	}
}

func TestEmptyAndSingleItemList(t *testing.T) {
	empty, err := gogenconf.ParseExpr("[]")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := empty.(gogenconf.List)
	if !ok || len(list.Items) != 0 {
		t.Fatalf("expected empty list, got %v", empty)
	}

	single, err := gogenconf.ParseExpr(`[ "only" ]`)
	if err != nil {
		t.Fatal(err)
	}
	sList, ok := single.(gogenconf.List)
	if !ok || len(sList.Items) != 1 {
		t.Fatalf("expected 1-item list, got %v", single)
	}
}

func TestNestedList(t *testing.T) {
	input := `[ [ 1, 2 ], [ 3, 4 ] ]`
	e, err := gogenconf.ParseExpr(input)
	if err != nil {
		t.Fatal(err)
	}
	list, ok := e.(gogenconf.List)
	if !ok || len(list.Items) != 2 {
		t.Fatalf("expected outer list with 2 items, got %v", e)
	}

	inner0, ok := list.Items[0].(gogenconf.List)
	if !ok || len(inner0.Items) != 2 {
		t.Fatalf("expected inner list 0, got %v", list.Items[0])
	}
	inner1, ok := list.Items[1].(gogenconf.List)
	if !ok || len(inner1.Items) != 2 {
		t.Fatalf("expected inner list 1, got %v", list.Items[1])
	}
}

func TestMapExprParseAndFormat(t *testing.T) {
	input := `{ host: "127.0.0.1", port: 8080, "Content-Type": "application/json" }`
	e, err := gogenconf.ParseExpr(input)
	if err != nil {
		t.Fatalf("ParseExpr(%q) failed: %v", input, err)
	}

	mp, ok := e.(gogenconf.Map)
	if !ok {
		t.Fatalf("expected Map, got %T", e)
	}

	if len(mp.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(mp.Entries))
	}

	if mp.Entries[0].Key != "host" || mp.Entries[0].Value.(gogenconf.Literal).Value != "127.0.0.1" {
		t.Errorf("entry 0 mismatch: %v", mp.Entries[0])
	}
	if mp.Entries[1].Key != "port" || mp.Entries[1].Value.(gogenconf.Literal).Value != "8080" {
		t.Errorf("entry 1 mismatch: %v", mp.Entries[1])
	}
	if mp.Entries[2].Key != "Content-Type" || mp.Entries[2].Value.(gogenconf.Literal).Value != "application/json" {
		t.Errorf("entry 2 mismatch: %v", mp.Entries[2])
	}

	formatted := gogenconf.FormatExpr(mp)
	e2, err := gogenconf.ParseExpr(formatted)
	if err != nil {
		t.Fatalf("re-parsing formatted map failed: %v", err)
	}
	mp2, ok := e2.(gogenconf.Map)
	if !ok || len(mp2.Entries) != 3 {
		t.Fatalf("unexpected re-parsed map: %v", e2)
	}
}

func TestEmptyMap(t *testing.T) {
	empty, err := gogenconf.ParseExpr("{}")
	if err != nil {
		t.Fatal(err)
	}
	mp, ok := empty.(gogenconf.Map)
	if !ok || len(mp.Entries) != 0 {
		t.Fatalf("expected empty map, got %v", empty)
	}
}

func TestMapOfLists(t *testing.T) {
	input := `{ allowed: [ "a", "b" ], denied: [ "c" ] }`
	e, err := gogenconf.ParseExpr(input)
	if err != nil {
		t.Fatal(err)
	}
	mp, ok := e.(gogenconf.Map)
	if !ok || len(mp.Entries) != 2 {
		t.Fatalf("expected map with 2 entries, got %v", e)
	}

	list0, ok := mp.Entries[0].Value.(gogenconf.List)
	if !ok || len(list0.Items) != 2 {
		t.Fatalf("expected list of 2 items in entry 0, got %v", mp.Entries[0].Value)
	}
}

func TestDocumentCollections(t *testing.T) {
	input := `config for myapp version 1

section network
    interfaces [ "eth0", "lo" ]
    routes { default: "192.168.1.1", internal: "10.0.0.1" }
end
`

	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "net.conf")
	if err != nil {
		t.Fatalf("ParseNamed failed: %v", err)
	}

	interfacesVal := doc.PathValue("network", "interfaces")
	list, ok := interfacesVal.(gogenconf.List)
	if !ok || len(list.Items) != 2 {
		t.Fatalf("expected 2 interfaces in list, got %v", interfacesVal)
	}

	routesVal := doc.PathValue("network", "routes")
	mp, ok := routesVal.(gogenconf.Map)
	if !ok || len(mp.Entries) != 2 {
		t.Fatalf("expected 2 routes in map, got %v", routesVal)
	}

	// Verify deep cloning
	cloned := doc.Clone()
	clonedList := cloned.PathValue("network", "interfaces").(gogenconf.List)
	clonedList.Items[0] = gogenconf.Literal{Value: "wlan0"}

	origList := doc.PathValue("network", "interfaces").(gogenconf.List)
	if origList.Items[0].(gogenconf.Literal).Value != "eth0" {
		t.Fatalf("mutating clone modified original: got %v", origList.Items[0])
	}

	// Verify round-trip formatting
	formatted := gogenconf.Format(doc)
	doc2, err := gogenconf.ParseNamed(strings.NewReader(formatted), "formatted.conf")
	if err != nil {
		t.Fatalf("re-parsing formatted document failed: %v\nOutput:\n%s", err, formatted)
	}

	doc2Interfaces := doc2.PathValue("network", "interfaces").(gogenconf.List)
	if len(doc2Interfaces.Items) != 2 || doc2Interfaces.Items[0].(gogenconf.Literal).Value != "eth0" {
		t.Fatalf("formatted document mismatch: %v", doc2Interfaces)
	}
}

func TestCollectionSyntaxErrors(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		errSub string
	}{
		{"unterminated list", `[ "a", "b"`, "unterminated list"},
		{"unterminated map", `{ "k": "v"`, "unterminated map"},
		{"malformed list missing comma", `[ "a" "b" ]`, "expected ',' or ']'"},
		{"missing map key", `{ : "val" }`, "expected map key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gogenconf.ParseExpr(tc.input)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("expected error containing %q, got %v", tc.errSub, err)
			}
		})
	}
}
