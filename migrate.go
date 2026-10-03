package configmodel

import "fmt"

type Migration func(*Document) error
type Migrations struct{ steps map[int]Migration }

func NewMigrations() *Migrations { return &Migrations{steps: map[int]Migration{}} }
func (m *Migrations) Register(from int, step Migration) error {
	if from < 0 || step == nil {
		return fmt.Errorf("invalid migration registration")
	}
	if m.steps[from] != nil {
		return fmt.Errorf("migration from version %d already registered", from)
	}
	m.steps[from] = step
	return nil
}
func (m *Migrations) Apply(d *Document, target int) error {
	version := d.Version
	if !d.HasVersion {
		version = 0
	}
	if version > target {
		return fmt.Errorf("configuration version %d is newer than supported version %d", version, target)
	}
	for version < target {
		step := m.steps[version]
		if step == nil {
			return fmt.Errorf("no migration from version %d to %d", version, version+1)
		}
		next := d.Clone()
		if err := step(next); err != nil {
			return fmt.Errorf("migrate version %d to %d: %w", version, version+1, err)
		}
		version++
		next.Version = version
		next.HasVersion = true
		*d = *next
	}
	return nil
}
