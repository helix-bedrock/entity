package state

import (
	"encoding/json"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// Seat is one of minecraft:rideable's seats. The engine places riders.
type Seat struct {
	MinRiderCount, MaxRiderCount int32
	Position                     behavior.Vec3
	LockRiderRotation            float32
	// RotateRiderBy stays as written: a number or a Molang expression.
	RotateRiderBy json.RawMessage
}

// RideableConfig is minecraft:rideable.
type RideableConfig struct {
	SeatCount, ControllingSeat int32
	// CrouchingSkipInteract stops a sneaking player from mounting.
	CrouchingSkipInteract, PullInEntities, RiderCanInteract bool
	// FamilyTypes are the families that may ride; empty lets any entity.
	FamilyTypes []string
	// PassengerMaxWidth is the widest passenger, 0 for no limit.
	PassengerMaxWidth         float32
	InteractText              string
	OnRiderEnter, OnRiderExit *definition.Trigger
	DismountMode              string
	Seats                     []Seat
}

// DefaultRideable is BDS's rideable (1.26.60 schema 0x744CFB0:
// seat_count 1, controlling_seat 0, crouching_skip_interact true,
// pull_in_entities and rider_can_interact false).
func DefaultRideable() RideableConfig {
	return RideableConfig{SeatCount: 1, CrouchingSkipInteract: true}
}

// Rideable is minecraft:rideable: who may ride the mob (1.21.3
// RideableComponent: getInteraction 0x140637150, allowInteraction
// 0x140629070, pullInEntity 0x14063C4D0, fitsInVehicle 0x1406327C0, and
// VehicleStateProvider::canAddPassenger 0x14062FE60). Riding itself is the
// engine's.
type Rideable struct{ cfg RideableConfig }

// NewRideable makes the component.
func NewRideable(c RideableConfig) *Rideable { return &Rideable{cfg: c} }

// Config returns the settings.
func (r *Rideable) Config() RideableConfig { return r.cfg }

// SetConfig changes the settings.
func (r *Rideable) SetConfig(c RideableConfig) { r.cfg = c }

// CanAddPassenger reports that p may ride the mob: the mob alive and not
// removed, p not the mob, p's own test, a free seat, p not already
// riding it, and p of one of family_types when any are listed.
func (r *Rideable) CanAddPassenger(env *Env, p behavior.Actor) bool {
	v := env.Self
	if p == nil || v.Removed() || !v.Alive() || p.ID() == v.ID() {
		return false
	}
	if env.CanRide != nil && !env.CanRide(p, v) {
		return false
	}
	riders := v.Riders()
	if int32(len(riders)) >= r.cfg.SeatCount {
		return false
	}
	for _, id := range riders {
		if id == p.ID() {
			return false
		}
	}
	if len(r.cfg.FamilyTypes) == 0 {
		return true
	}
	if env.HasFamily == nil {
		return false
	}
	for _, f := range r.cfg.FamilyTypes {
		if env.HasFamily(p, f) {
			return true
		}
	}
	return false
}

// Interact reports that a player may mount the mob by interacting:
// crouching_skip_interact turns sneaking players away, and inputLocked is
// the engine's answer to the player's mount input lock.
func (r *Rideable) Interact(env *Env, player behavior.Actor, sneaking, inputLocked bool) bool {
	if r.cfg.CrouchingSkipInteract && sneaking {
		return false
	}
	return !inputLocked && r.CanAddPassenger(env, player)
}

// FitsInVehicle reports a box no wider than passenger_max_width.
func (r *Rideable) FitsInVehicle(b behavior.Box) bool {
	return r.cfg.PassengerMaxWidth == 0 || r.cfg.PassengerMaxWidth > b.Max.X-b.Min.X
}

// PullIn reports that the mob pulls a touching entity in as a rider, with
// pull_in_entities: pullable (the engine's answer), fitting, not riding
// or leashed, and allowed by CanAddPassenger. The engine starts the ride.
func (r *Rideable) PullIn(env *Env, p behavior.Actor, pullable bool) bool {
	if !r.cfg.PullInEntities || p == nil || !pullable || !r.FitsInVehicle(p.Box()) {
		return false
	}
	if p.Vehicle() != behavior.NoID || p.Flag(behavior.FlagLeashed) {
		return false
	}
	return r.CanAddPassenger(env, p)
}

// Rider is one of minecraft:addrider's riders: an entity the engine spawns
// riding the mob, with an event fired on it.
type Rider struct {
	EntityType, SpawnEvent string
}

// AddRiderConfig is minecraft:addrider. BDS spawns the riders when the
// component is added (AddRiderComponent::reloadComponent 0x14056E000); the
// engine does that from this list.
type AddRiderConfig struct{ Riders []Rider }

// TransformationConfig is minecraft:transformation. The new entity, its
// equipment, inventory, level, owner, sounds and the cure discounts are
// the engine's; entity counts the delay down.
type TransformationConfig struct {
	Into string
	// Delay is in ticks, plus a draw in [RangeMin, RangeMax).
	Delay, RangeMin, RangeMax int32
	// BlockAssistChance is the chance each tick to look for BlockTypes
	// within BlockRadius; each of up to BlockMax found speeds the delay by
	// one tick with chance BlockChance.
	BlockAssistChance, BlockChance           float32
	BlockRadius, BlockMax                    int32
	BlockTypes                               []string
	DropEquipment, DropInventory             bool
	PreserveEquipment, KeepLevel, KeepOwner  bool
	BeginTransformSound, TransformationSound []string
	// Add is the add.component_groups list for the new entity.
	Add []string
}

// Transformation is minecraft:transformation (1.21.3
// _tickTransformationComponent 0x14015E980,
// TransformationComponent::reloadComponent 0x14063DD00, transformIfAble
// 0x140645B70, TransformationDescription::deserializeData 0x1406322E0, all
// defaults 0).
type Transformation struct {
	cfg   TransformationConfig
	timer int32
}

// FlagTransforming is the actor flag raised while a transformation runs.
const FlagTransforming = 55

// NewTransformation makes the component, raises the transforming flag and
// draws the delay.
func NewTransformation(c TransformationConfig, m Mob, r behavior.Random) *Transformation {
	t := &Transformation{cfg: c}
	if m != nil {
		m.SetFlag(FlagTransforming, true)
	}
	d := c.RangeMin
	if r != nil {
		d = randRange(r, c.RangeMin, c.RangeMax)
	}
	t.timer = d + c.Delay
	return t
}

// Config returns the settings.
func (t *Transformation) Config() TransformationConfig { return t.cfg }

// SetConfig changes the settings; the timer is kept.
func (t *Transformation) SetConfig(c TransformationConfig) { t.cfg = c }

// Timer is the ticks left.
func (t *Transformation) Timer() int32 { return t.timer }

// Tick counts the delay down, faster near assisting blocks, and reports
// that the mob should transform now. The engine then transforms it.
func (t *Transformation) Tick(env *Env) bool {
	step := int32(1)
	r := env.World.Random()
	if r.NextFloat() < t.cfg.BlockAssistChance && env.BlockMatches != nil {
		step += t.assist(env, r)
	}
	t.timer -= step
	return t.timer <= 0 && !env.Self.Removed()
}

// assist counts the assisting blocks around the mob in BDS's order: x, y,
// z from the truncated position minus the radius, stopping at block_max.
func (t *Transformation) assist(env *Env, r behavior.Random) int32 {
	p := env.Self.Pos()
	rad := t.cfg.BlockRadius
	x0, y0, z0 := int32(p.X), int32(p.Y), int32(p.Z)
	var found, extra int32
	for x := x0 - rad; x < x0+rad && found < t.cfg.BlockMax; x++ {
		for y := y0 - rad; y < y0+rad && found < t.cfg.BlockMax; y++ {
			for z := z0 - rad; z < z0+rad && found < t.cfg.BlockMax; z++ {
				if !env.BlockMatches(behavior.Pos{X: x, Y: y, Z: z}, t.cfg.BlockTypes) {
					continue
				}
				if r.NextFloat() < t.cfg.BlockChance {
					extra++
				}
				found++
			}
		}
	}
	return extra
}
