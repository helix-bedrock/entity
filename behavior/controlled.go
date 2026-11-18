package behavior

import "math"

// The controlled_by_player numbers.
const (
	rideAirFriction    = 0.91 // the friction off the ground
	rideFrictionScale  = 0.91 // the block friction share on the ground
	rideFrictionNorm   = 0.16277139271896832
	rideFrictionMin    = 0.00000011920929
	rideMoveMin2       = 0.0001 // squared move below which the mob does not move
	rideStepMax        = 1      // highest step the mob jumps up
	rideHalf           = 0.5
	rideFrictionFactor = 3
)

// PlayerControlled is the part of the mob controlled_by_player needs on top
// of Mob.
type PlayerControlled interface {
	Mob
	// ControlItems are the item_controllable component's control_items; ok
	// is false without the component.
	ControlItems() (items []string, ok bool)
	// ItemMatches reports the stack matching an item descriptor, as the
	// engine reads it (BDS's item names and their aliases).
	ItemMatches(descriptor string, it Item) bool
	// HeldItem is the player's selected item.
	HeldItem(id ID) Item
	// Yaw is ActorRotationComponent's yaw; YawOf is another entity's.
	Yaw() float32
	YawOf(id ID) float32
	// SetSprinting is.
	SetSprinting(on bool)
	// SetSpeed is.
	SetSpeed(s float32)
	// ResetMovement sets the movement attribute's current value back to
	// its base.
	ResetMovement()
	// RideFriction is the friction of the block at p: BDS uses 0.6 for one
	// block (BDS 1.26.60 qword, taken to be air) and the block's own friction
	// for the others.
	RideFriction(p Pos) float32
	// MaxAutoStep is the MaxAutoStepComponent height; ok is false without
	// it.
	MaxAutoStep() (h float32, ok bool)
	// WalkThrough is BDS 1.26.60: the block at p is one the mob passes (a
	// block with the type bit and two door-like states).
	WalkThrough(p Pos) bool
	// Jump asks for a jump this tick (the component 0x2F50F705 flag).
	Jump()
	// MoveRelativeSpeed is the move speed for the mob's travel type
	// (BDS 1.26.60 of the travel type).
	MoveRelativeSpeed() float32
}

// ControlledByPlayerConfig is the JSON of
// minecraft:behavior.controlled_by_player.
type ControlledByPlayerConfig struct {
	FractionalRotation      float32 `json:"fractional_rotation"`
	FractionalRotationLimit float32 `json:"fractional_rotation_limit"`
	MountSpeedMultiplier    float32 `json:"mount_speed_multiplier"`
}

// DefaultControlledByPlayer returns the goal's BDS defaults.
func DefaultControlledByPlayer() ControlledByPlayerConfig {
	return ControlledByPlayerConfig{FractionalRotation: 0.5, FractionalRotationLimit: 5, MountSpeedMultiplier: 1}
}

// ControlledByPlayer is ControlledByPlayerGoal: a player riding the mob
// with a control item (a carrot on a stick) steers it where they look.
type ControlledByPlayer struct {
	base
	m   PlayerControlled
	cfg ControlledByPlayerConfig
}

// NewControlledByPlayer makes the goal. It has no control flags.
func NewControlledByPlayer(w World, m PlayerControlled, c ControlledByPlayerConfig) *ControlledByPlayer {
	return &ControlledByPlayer{base: base{mob: m, world: w}, m: m, cfg: c}
}

// CanUse implements goal.Goal: a player rides the mob and one
// player rider holds a control item.
func (g *ControlledByPlayer) CanUse() bool {
	m := g.m
	rider := false
	for _, id := range m.Riders() {
		if a := g.actor(id); a != nil && !a.Removed() && a.IsPlayer() {
			rider = true
			break
		}
	}
	if !rider || !m.Alive() {
		return false
	}
	items, ok := m.ControlItems()
	if !ok {
		return false
	}
	for _, id := range m.Riders() {
		a := g.actor(id)
		if a == nil || !a.IsPlayer() {
			continue
		}
		it := m.HeldItem(id)
		if it.Empty() {
			continue
		}
		for _, d := range items {
			if m.ItemMatches(d, it) {
				return true
			}
		}
	}
	return false
}

// CanContinueToUse implements goal.Goal: BDS asks canUse again.
func (g *ControlledByPlayer) CanContinueToUse() bool { return g.CanUse() }

// Start implements goal.Goal.
func (g *ControlledByPlayer) Start() {
	g.m.ResetMovement()
	g.m.SetSpeed(g.m.Speed())
}

// Stop implements goal.Goal.
func (g *ControlledByPlayer) Stop() { g.m.SetSprinting(false) }

// Tick implements goal.Goal: the mob turns part of the way to
// the rider's yaw, jumps a step in its way and moves forward.
func (g *ControlledByPlayer) Tick() {
	m := g.m
	var rider Actor
	// BDS keeps the last rider's player lookup, so a last rider that is not
	// a player stops the steering.
	for _, id := range m.Riders() {
		rider = nil
		if a := g.actor(id); a != nil && a.IsPlayer() {
			rider = a
		}
	}
	if rider == nil || rider.Removed() {
		return
	}
	yaw := m.Yaw()
	turn := wrapDegrees(m.YawOf(rider.ID())-yaw) * g.cfg.FractionalRotation
	if lim := g.cfg.FractionalRotationLimit; lim < turn {
		turn = lim
	} else {
		turn = max(turn, -lim)
	}
	yaw = wrapDegrees(turn + yaw)
	m.SetYaw(yaw)

	speed := m.Speed() * g.cfg.MountSpeedMultiplier
	g.stepUp(yaw, speed)

	f := m.MoveRelativeSpeed()
	if d2 := speed*speed + 0; d2 >= rideMoveMin2 {
		fwd := speed * (f / max(1, float32(math.Sqrt(float64(d2)))))
		s, c := math.Sincos(float64(yaw * radPerDeg))
		v := m.Velocity()
		m.SetVelocity(Vec3{X: v.X - fwd*float32(s), Y: v.Y, Z: v.Z + fwd*float32(c)})
	}
}

// friction is the tick's ground friction factor.
func (g *ControlledByPlayer) friction() float32 {
	if !g.m.OnGround() {
		return rideAirFriction
	}
	return g.m.RideFriction(blockOf(g.m.Pos())) * rideFrictionScale
}

// stepUp jumps when the block half a width ahead is a step between the
// auto step height and a full block.
func (g *ControlledByPlayer) stepUp(yaw, speed float32) {
	m := g.m
	var k float32
	if fr := g.friction(); abs32(fr) >= rideFrictionMin {
		k = float32(rideFrictionNorm / math.Pow(float64(fr), rideFrictionFactor))
	}
	a := yaw * radPerDeg * sinScale
	sin, cos := tableSin(uint16(int32(a))), tableSin(uint16(int32(a+sinQuarter)))
	move := k * speed / max(speed, 1) * speed
	dx, dz := -sin*move, cos*move
	half := boxWidth(m.Box()) * rideHalf
	var ox, oz float32
	if abs32(dx) <= abs32(dz) {
		oz = signedHalf(dz, half) + dz
	} else {
		ox = signedHalf(dx, half) + dx
	}
	p := m.Pos()
	ahead := blockOf(Vec3{X: p.X + ox, Y: p.Y, Z: p.Z + oz})
	if m.WalkThrough(ahead) || ahead.X == floorInt(p.X) && ahead.Z == floorInt(p.Z) {
		return
	}
	var top float32
	if b := g.world.Block(ahead); b.HasCollision {
		top = float32(ahead.Y) + b.ShapeTop
	}
	step := top - m.Box().Min.Y
	if h, ok := m.MaxAutoStep(); ok && step <= rideStepMax && step > h {
		m.Jump()
	}
}

// signedHalf is half the width with the sign of v; 0 counts as positive.
func signedHalf(v, half float32) float32 {
	if v < 0 {
		return -half
	}
	return half
}
