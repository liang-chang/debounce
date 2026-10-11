//revive:disable:var-naming
package types

// Context type tracks top level debugging flag.
type Context struct {
	Debug   bool
	Success bool
}

type DebounceCommand struct {
	Quantity string
	Unit     string
	Key      string
	Command  []string
	Debug    bool
	Local    bool
	Status   bool
}
