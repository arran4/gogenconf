package schema

import "github.com/arran4/gogenconf"

func Definition() gogenconf.Schema {
	return gogenconf.Schema{Version: 1, Sections: []gogenconf.SectionDefinition{{Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []gogenconf.Field{
		{Key: "endpoint", GoName: "Endpoint", GoType: "string", Default: gogenconf.Literal{Value: "https://example.test"}, Documentation: []string{"Remote endpoint."}},
		{Key: "credential", GoName: "Credential", GoType: "[]byte", Required: true, Sensitive: true, Documentation: []string{"Credential source."}, Examples: []gogenconf.Expr{gogenconf.Call{Name: "from_file", Args: []gogenconf.Expr{gogenconf.Call{Name: "from_env", Args: []gogenconf.Expr{gogenconf.Literal{Value: "CREDENTIAL_FILE"}}}}}}},
	}}}}
}
