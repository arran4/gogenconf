package schema

import "github.com/arran4/gogenconf"

func Definition() gogenconf.Schema {
	return gogenconf.Schema{
		Version: 2,
		Sections: []gogenconf.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Documentation: []string{
					"Service network and security configuration.",
				},
				Fields: []gogenconf.Field{
					{
						Key:           "endpoint",
						GoName:        "Endpoint",
						GoType:        "string",
						Default:       gogenconf.Literal{Value: "https://api.internal.net"},
						Documentation: []string{"Remote endpoint for API operations."},
					},
					{
						Key:           "api_key",
						GoName:        "APIKey",
						GoType:        "string",
						Required:      true,
						Sensitive:     true,
						Documentation: []string{"API authentication key."},
						Examples: []gogenconf.Expr{
							gogenconf.Call{Name: "from_env", Args: []gogenconf.Expr{gogenconf.Literal{Value: "API_KEY"}}},
						},
					},
					{
						Key:           "legacy_token",
						InputOnly:     true,
						Deprecated:    true,
						Documentation: []string{"Legacy authentication token from version 1."},
					},
				},
			},
			{
				Name:     "worker",
				GoName:   "Workers",
				GoType:   "WorkerConfig",
				Repeated: true,
				ExampleNames: []string{
					"worker-1",
				},
				Documentation: []string{
					"Background worker instances.",
				},
				Fields: []gogenconf.Field{
					{
						Key:           "concurrency",
						GoName:        "Concurrency",
						GoType:        "string",
						Default:       gogenconf.Literal{Value: "4"},
						Documentation: []string{"Worker concurrency limit."},
					},
				},
			},
		},
	}
}
