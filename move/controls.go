package move

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// Jump is the jump control (minecraft:jump.static or jump.dynamic).
type Jump struct {
	// Jumping asks the mob to jump this tick.
	Jumping bool
}

func (b *Body) jump() {
	if b.Jump != nil {
		b.Jump.Jumping = true
	}
}

// source is the body whose position a riding mob moves from: its vehicle.
func (b *Body) source() *Body {
	if b.Vehicle != nil {
		return b.Vehicle
	}
	return b
}

// fly3D is the base move control's motion for mobs that do not walk: it
// pushes the velocity towards the target and faces the target or the
// direction of travel. It returns false when the mob arrived.
func fly3D(b *Body, c *Control, dx, dy, dz, d2 float32) {
	d := sqrt(d2)
	if avgSize(b.Box) > d {
		c.HasWanted = false
		damp(b, 0.5)
		return
	}
	inv := 1 / d
	b.Vel.Y += dy * inv * 0.05 * c.Speed
	b.Vel.Z += c.Speed * (dz * inv * 0.05)
	b.Vel.X += c.Speed * (dx * inv * 0.05)
	var target float32
	if b.Target != nil {
		target = atan2(b.Target.X-b.Pos.X, b.Target.Z-b.Pos.Z) * -degrees
	} else {
		target = atan2(b.Vel.X, b.Vel.Z) * -degrees
	}
	b.faceYaw(approach(b.Yaw, target, c.MaxTurn))
}

// Basic is minecraft:movement.basic.
func Basic(b *Body, c *Control) {
	if b.Flags.Has(FlagHasGravity) {
		b.LocalMove.Z = 0
	}
	if !c.HasWanted {
		b.MoveSpeed = 0
		return
	}
	src := b.source()
	dx, dz := c.Wanted.X-src.Pos.X, c.Wanted.Z-src.Pos.Z
	dy := c.Wanted.Y - src.Box.Min.Y
	d2 := (dx*dx + dy*dy) + dz*dz
	if b.Flags.Has(FlagCharging) || !b.Flags.Has(FlagHasGravity) {
		fly3D(b, c, dx, dy, dz, d2)
		return
	}
	if d2 < 0.1 && dy < 0.25 {
		c.HasWanted = false
		return
	}
	s := b.MovementSpeed * c.Speed
	b.Yaw = turnYaw(b, c.MaxTurn, dx, dz, s)
	b.SetSpeed(moveSpeed(b, c.Wanted, s))
	step := b.MaxAutoStep
	if b.WasInWater {
		step = 0.009
	}
	if dy > step && max32(b.Width, 1) > dx*dx+dz*dz {
		b.jump()
	}
}

// Generic is minecraft:movement.generic: walking that steps onto slabs
// without jumping, and swimming in three dimensions.
func Generic(w path.World, b *Body, c *Control) {
	if b.Flags.Has(FlagHasGravity) {
		b.LocalMove.Y, b.LocalMove.Z = 0, 0
	}
	if !c.HasWanted {
		b.MoveSpeed = 0
		return
	}
	if b.Flags.Has(FlagSwimming) {
		swim3D(b, c)
		return
	}
	b.LocalMove.Y = 0
	src := b.source()
	dy := c.Wanted.Y - src.Box.Min.Y
	dx, dz := c.Wanted.X-src.Pos.X, c.Wanted.Z-src.Pos.Z
	at := path.Pos{X: floorInt(c.Wanted.X), Y: floorInt(c.Wanted.Y), Z: floorInt(c.Wanted.Z)}
	box := w.CollisionBox(w.Block(at), at)
	h := box.Max.Y - box.Min.Y
	if h > 0 && h < 1 {
		if h <= dy {
			dy -= h
		} else {
			dy += h
		}
	}
	d2 := (dx*dx + dy*dy) + dz*dz
	if b.Flags.Has(FlagCharging) || !b.Flags.Has(FlagHasGravity) {
		fly3D(b, c, dx, dy, dz, d2)
		return
	}
	if d2 < 0.1 {
		c.HasWanted = false
		return
	}
	s := b.MovementSpeed * c.Speed
	b.Yaw = turnYaw(b, c.MaxTurn, dx, dz, s)
	b.SetSpeed(moveSpeed(b, Vec3{X: c.Wanted.X, Y: h + c.Wanted.Y, Z: c.Wanted.Z}, s))
	if dy > 0.009 && max32(b.Width, 1) > dx*dx+dz*dz {
		b.jump()
	}
}

// swim3D is the generic control's swimming: it turns, pitches the move
// input towards the target and looks where it swims.
func swim3D(b *Body, c *Control) {
	dx, dy, dz := c.Wanted.X-b.Pos.X, c.Wanted.Y-b.Pos.Y, c.Wanted.Z-b.Pos.Z
	dist := sqrt((dy*dy + dx*dx) + dz*dz)
	if dist < 0.00000025000003 {
		return
	}
	b.faceYaw(approach(b.Yaw, atan2(dz, dx)*degrees-90, c.MaxTurn))
	s := b.MovementSpeed * c.Speed
	b.SetSpeed(moveSpeed(b, c.Wanted, s))
	p := atan2(dy, sqrt(dx*dx+dz*dz)) * degrees * -0.017453292
	b.LocalMove.Y = -float32(math.Sin(float64(p))) * s
	b.LocalMove.Z = float32(math.Cos(float64(p))) * s
	if b.InWater {
		b.Vel.Y += 0.02
	}
	if l := b.Look; l != nil && l.Mode == 0 {
		t := Vec3{X: dx/dist + dx/dist + b.Pos.X, Y: b.Eye.Y + dy/dist, Z: dz/dist + dz/dist + b.Pos.Z}
		prev := t
		if l.Has {
			prev = l.Pos
		}
		l.Has = true
		l.Pos = Vec3{X: (t.X-prev.X)*0.125 + prev.X, Y: (t.Y-prev.Y)*0.125 + prev.Y, Z: (t.Z-prev.Z)*0.125 + prev.Z}
		l.YawSpeed, l.PitchSpeed = 10, 40
	}
	b.Flags.Set(FlagMoving, true)
}

// Fly is minecraft:movement.fly.
func Fly(b *Body, c *Control) {
	if !c.HasWanted {
		return
	}
	dx, dz := c.Wanted.X-b.Pos.X, c.Wanted.Z-b.Pos.Z
	dy := c.Wanted.Y - b.Box.Min.Y
	c.HasWanted = false
	d2 := dz*dz + (dx*dx + dy*dy)
	d := sqrt(d2)
	if avgSize(b.Box) > d {
		damp(b, 0.25)
	}
	attr := b.MovementSpeed
	if d2 != 0 {
		inv := 1 / d
		b.Vel.Y += dy * inv * 0.05 * c.Speed * attr
		b.Vel.Z += attr * (c.Speed * (dz * inv * 0.05))
		b.Vel.X += attr * (c.Speed * (dx * inv * 0.05))
	}
	b.faceYaw(atan2(b.Vel.X, b.Vel.Z) * -degrees)
}

// Hover is minecraft:movement.hover: the mob floats without gravity, aims
// its box's centre at the target and brakes near it.
func Hover(b *Body, c *Control) {
	b.Flags.Set(FlagHasGravity, false)
	b.OnGround = false
	b.FallDistance = 0
	ctr := boxCentre(b.Box)
	dx, dy, dz := c.Wanted.X-ctr.X, c.Wanted.Y-ctr.Y, c.Wanted.Z-ctr.Z
	b.LocalMove = Vec3{}
	a := avgSize(b.Box)
	f := float32(0.95)
	if a*a > dx*dx+dz*dz {
		f = 0.35
	}
	damp(b, f)
	if !c.HasWanted {
		b.MoveSpeed = 0
		return
	}
	if dz*dz+(dx*dx+dy*dy) < 0.01 {
		c.HasWanted = false
		return
	}
	sp := b.AirSpeed
	if sp <= 0 {
		return
	}
	s := sp * c.Speed
	b.SetSpeed(moveSpeed(b, c.Wanted, s))
	b.Yaw = turnYaw(b, c.MaxTurn, dx, dz, s)
	var up, fwd float32
	if dy*dy > 0.0001 {
		up = min(abs32(dy), s)
		if dy < 0 {
			up = -up
		}
	}
	if dx*dx+dz*dz > 0.0001 {
		fwd = b.Speed
	}
	b.LocalMove.Y, b.LocalMove.Z = up, fwd
}

// Sway is minecraft:movement.sway's settings.
type Sway struct {
	Frequency, Amplitude float32
}

// Swimmer is what minecraft:movement.swim reads besides the body.
type Swimmer struct {
	Sway *Sway // nil without sway
	// Ticks is the mob's age in ticks and ID its unique ID: together they
	// set where in its sway the mob is.
	Ticks int32
	ID    int64
}

// Swim is minecraft:movement.swim.
func Swim(b *Body, c *Control, sw *Swimmer) {
	if !c.HasWanted || c.Wanted == (Vec3{}) {
		b.SetSpeed(0)
		b.LocalMove = Vec3{}
		b.Flags.Set(FlagMoving, false)
		return
	}
	c.HasWanted = false
	dx, dy, dz := c.Wanted.X-b.Pos.X, c.Wanted.Y-b.Pos.Y, c.Wanted.Z-b.Pos.Z
	dist := sqrt((dx*dx + dy*dy) + dz*dz)
	if dist < 0.0005 {
		return
	}
	if dx*dx+dz*dz >= 0.05 {
		b.faceYaw(approach(b.Yaw, atan2(dz, dx)*degrees-90, c.MaxTurn))
	}
	if sw != nil && sw.Sway != nil && sw.Sway.Amplitude > 0 && b.WasInWater {
		off := int32(sw.ID) % 100
		if off < 0 {
			off = -off
		}
		t := float32(sw.Ticks) + float32(off)
		a := tableSin(sw.Sway.Frequency*t) * sw.Sway.Amplitude
		yr := b.Yaw * 0.017453292 * sinScale
		cy, sy := sinTable[uint16(int32(yr+16384))], sinTable[uint16(int32(yr))]
		b.Vel.X += a * cy
		b.Vel.Z += a * sy
		b.Vel.Y += (cy + sy) * (tableSin(t*0.75) * 0.05) * 0.5
		b.Vel.Y += b.Speed * dy * 0.1
	}
	s := b.MovementSpeed * c.Speed
	b.SetSpeed(s)
	b.Flags.Set(FlagMoving, true)
	p := b.Pitch * 0.017453292
	sn := float32(math.Sin(float64(p)))
	m := min(abs32(dy*5), 1.25)
	m = max32(abs32(sn), m)
	var sign float32
	switch {
	case sn < 0:
		sign = 1
	case sn > 0:
		sign = -1
	}
	b.LocalMove.Y = sign * m * s
	b.LocalMove.Z = float32(math.Cos(float64(p))) * s
	if b.InWater {
		b.Vel.Y += 0.02
	}
	if l := b.Look; l != nil {
		t := Vec3{X: dx/dist + dx/dist + b.Pos.X, Y: b.Eye.Y + dy/dist, Z: dz/dist + dz/dist + b.Pos.Z}
		pz := t.Z
		if l.Has {
			pz = l.Pos.Z
		}
		l.Mode = 2
		l.Has = true
		// BDS smooths only the z of the look target here.
		l.Pos = Vec3{X: t.X, Y: t.Y, Z: (t.Z-pz)*0.125 + pz}
		l.YawSpeed, l.PitchSpeed = 10, 40
	}
}

// Amphibious is minecraft:movement.amphibious: the basic control on land,
// and swimming along the path in water.
func Amphibious(b *Body, c *Control, pathActive bool, underwaterSpeed, height float32) {
	if !b.WasInWater {
		Basic(b, c)
		return
	}
	if !c.HasWanted || !pathActive {
		b.SetSpeed(0)
		return
	}
	half := height * 0.5
	dx := c.Wanted.X - b.Pos.X
	dy := c.Wanted.Y - b.Box.Min.Y
	dz := c.Wanted.Z - b.Pos.Z
	off := -half
	if dy > 0 {
		off = half
	}
	up := off + dy
	vy := up / sqrt(dz*dz+(dx*dx+up*up))
	y := approach(b.Yaw, atan2(dz, dx)*degrees-90, c.MaxTurn)
	b.BodyYaw = y
	b.Yaw = y
	b.SetSpeed(moveSpeed(b, c.Wanted, c.Speed*underwaterSpeed))
	b.Vel.Y += b.Speed * vy * 0.1
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func floorInt(v float32) int32 { return int32(math.Floor(float64(v))) }

// sinScale turns radians into a sine table index.
const sinScale = 10430.378

// sinTable is BDS's 65536-entry sine table (mce::Math).
var sinTable = func() (t [65536]float32) {
	for i := range t {
		t[i] = float32(math.Sin(float64(i) * math.Pi * 2 / 65536))
	}
	return t
}()

// tableSin is the table sine of x radians.
func tableSin(x float32) float32 { return sinTable[uint16(int32(x*sinScale))] }
