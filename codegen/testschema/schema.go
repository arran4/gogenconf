// Package testschema provides a small non-Address schema for generator fixtures.
package testschema

import m "github.com/arran4/address/internal/configmodel"

func Definition() m.Schema {
	return m.Schema{Version: 2, Sections: []m.SectionDefinition{
		{Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []m.Field{
			{Key: "label", GoName: "Label", GoType: "string", Default: m.Literal{Value: "default"}, Environment: "FIXTURE_LABEL", Required: true},
			{Key: "key", GoName: "Key", GoType: "[]byte", Required: true, Sensitive: true},
			{Key: "content", GoName: "Content", GoType: "[]byte", Required: true, Policy: m.ProviderBacked},
		}},
	}}
}
