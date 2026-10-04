package schema

import "github.com/arran4/gogenconf"

func Definition() gogenconf.Schema {
	return gogenconf.Schema{Version: 1, Sections: []gogenconf.SectionDefinition{{Name: "worker", GoName: "Workers", GoType: "WorkerConfig", Repeated: true, Fields: []gogenconf.Field{{Key: "label", GoName: "Label", GoType: "string", Required: true}}}}}
}
