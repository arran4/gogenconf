package schema

import "github.com/arran4/gogenconf"

func Definition() gogenconf.Schema {
	return gogenconf.Schema{Version: 1, Sections: []gogenconf.SectionDefinition{{Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []gogenconf.Field{{Key: "content", GoName: "Content", GoType: "[]byte", Required: true, Policy: gogenconf.ProviderBacked}}}}}
}
