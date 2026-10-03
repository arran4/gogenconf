package main

import (
	"fmt"
	"github.com/arran4/gogenconf"
	"strings"
)

func main() {
	d, err := gogenconf.Parse(strings.NewReader("config_version 1\nsection service\n # Operator choice\n old_endpoint https://private.example.test\n extension retain\nend\n"))
	if err != nil {
		panic(err)
	}
	m := gogenconf.NewMigrations()
	err = m.Register(1, func(d *gogenconf.Document) error {
		s := d.Section("service")
		s.Set("endpoint", gogenconf.CloneExpr(d.Value("service", "old_endpoint")))
		s.Delete("old_endpoint")
		return nil
	})
	if err != nil {
		panic(err)
	}
	if err = m.Apply(d, 2); err != nil {
		panic(err)
	}
	s := gogenconf.Schema{Version: 2, Sections: []gogenconf.SectionDefinition{{Name: "service", Fields: []gogenconf.Field{
		{Key: "endpoint", Default: gogenconf.Literal{Value: "https://default.example.test"}, Documentation: []string{"Current endpoint documentation."}},
		{Key: "retries", Default: gogenconf.Literal{Value: "3"}},
	}}}}
	if err = s.Enrich(d); err != nil {
		panic(err)
	}
	fmt.Print(gogenconf.Format(d))
}
