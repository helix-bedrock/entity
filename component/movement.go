package component

import (
	"fmt"
	"strings"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
	"github.com/helix-bedrock/entity/move"
	"github.com/helix-bedrock/entity/nav"
)

// The component name prefixes of navigation and movement.
const (
	NavigationPrefix = "minecraft:navigation."
	MovementPrefix   = "minecraft:movement."
)

// Navigation makes the navigation of the first minecraft:navigation.*
// component in order; ok is false when the mob has none. Vanilla files
// give a mob one at a time.
func Navigation(order []string, comps definition.Group) (n *nav.Navigation, name string, ok bool, err error) {
	for _, name := range order {
		k, isNav := nav.KindOf(name)
		if !isNav {
			continue
		}
		d, err := nav.ParseDescription(comps[name])
		return nav.NewKind(k, &d), name, true, err
	}
	return nil, "", false, nil
}

// Movement is a mob's minecraft:movement.* component.
type Movement struct {
	// Name is the component, such as minecraft:movement.basic.
	Name string
	// MaxTurn is max_turn in degrees per tick, 30 by default (1.26.60
	// 0x7133490).
	MaxTurn float32
	// Glider is movement.glide's state, nil for the other kinds.
	Glider *move.Glider
	// JumpDelay is movement.jump's jump_delay in seconds.
	JumpDelay behavior.Range[float32]
	// JSON is the component, for the members read elsewhere (sway).
	JSON Object
}

// The movement defaults: max_turn (1.26.60 0x7133490) and movement.glide's
// speeds (1.21.3 GlideMoveControlDescription::deserializeData 0x140631590).
const (
	defaultMaxTurn        = 30
	defaultGlideStart     = 0.1
	defaultGlideTurnSpeed = 0.2
	movementGlide         = MovementPrefix + "glide"
	movementJump          = MovementPrefix + "jump"
)

// ReadMovement reads the first minecraft:movement.* component in order; ok
// is false when the mob has none.
func ReadMovement(order []string, comps definition.Group) (m Movement, ok bool, err error) {
	for _, name := range order {
		if !strings.HasPrefix(name, MovementPrefix) {
			continue
		}
		j, perr := ParseObject(comps[name])
		var warnings []error
		c := &Context{Name: name, warn: func(e error) { warnings = append(warnings, e) }}
		m = Movement{Name: name, JSON: j, MaxTurn: c.Float(j, "max_turn", defaultMaxTurn)}
		switch name {
		case movementGlide:
			m.Glider = move.NewGlider(c.Float(j, "start_speed", defaultGlideStart), c.Float(j, "speed_when_turning", defaultGlideTurnSpeed))
		case movementJump:
			m.JumpDelay = c.Range(j, "jump_delay", m.JumpDelay)
		}
		if perr != nil {
			warnings = append(warnings, perr)
		}
		if len(warnings) > 0 {
			err = fmt.Errorf("component %s: %v", name, warnings)
		}
		return m, true, err
	}
	return Movement{}, false, nil
}
