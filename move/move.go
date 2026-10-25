// Package move turns a mob's wanted position into motion the way BDS's move
// controls (the minecraft:movement.* components) do: it turns the mob,
// sets its speed and move input, pushes its velocity and asks it to jump.
//
// Navigation (package nav) sets the wanted position; the host's physics then
// moves the mob from its velocity and move input.
package move

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// Vec3 is the pathfinder's position type.
type Vec3 = path.Vec3

// Box is the pathfinder's box type.
type Box = path.Box

// The actor flags the move controls read and write. The numbers are BDS's
// actor flag numbers.
const (
	FlagMoving     = 34
	FlagCharging   = 43
	FlagHasGravity = 49
	FlagSwimming   = 57
)

// Flags is a mob's set of actor flags.
type Flags [3]uint64

// Has reports whether flag n is set.
func (f *Flags) Has(n int) bool { return n >= 0 && n < 192 && f[n>>6]&(1<<(n&63)) != 0 }

// Set sets or clears flag n.
func (f *Flags) Set(n int, on bool) {
	if n < 0 || n >= 192 {
		return
	}
	if on {
		f[n>>6] |= 1 << (n & 63)
	} else {
		f[n>>6] &^= 1 << (n & 63)
	}
}

// Control is a mob's move control state: the wanted position navigation set
// and the move control's settings.
type Control struct {
	HasWanted bool
	Wanted    Vec3
	// MaxTurn is max_turn: the most the mob turns in a tick, in degrees.
	MaxTurn float32
	// Speed is the speed modifier navigation passed with the position.
	Speed float32
}

// SetWantedPosition makes the mob move to p at the speed modifier speed.
func (c *Control) SetWantedPosition(p Vec3, speed float32) {
	c.HasWanted, c.Wanted, c.Speed = true, p, speed
}

// ClearWanted drops the wanted position.
func (c *Control) ClearWanted() { c.HasWanted = false }

// Look is the look control's target.
type Look struct {
	Has                  bool
	Pos                  Vec3
	YawSpeed, PitchSpeed float32
	// Mode is 0 when nothing else drives the look; the swim control sets 2.
	Mode int32
	// HasYaw makes the look control turn to Yaw instead of Pos.
	HasYaw bool
	// Pitch is the look pitch the glide control sets with HasYaw.
	Pitch float32
	Yaw   float32
}

// Body is the part of a mob the move controls read and change. The host
// fills it before the tick and applies it after.
type Body struct {
	Pos, Vel Vec3
	Box      Box
	// Width is the collision shape's width (BDS keeps it apart from the box).
	Width float32
	// Pitch and Yaw are the mob's rotation; BodyYaw and HeadYaw its body and
	// head rotations.
	Pitch, Yaw, BodyYaw, HeadYaw float32
	Flags                        Flags

	OnGround, WasOnGround, WasInWater bool
	// InWater is BDS's "in water for swimming up" test, which also counts
	// bubble columns; the host answers it.
	InWater bool

	// MovementSpeed is the movement attribute's current value.
	MovementSpeed float32
	// AirSpeed is the speed a hovering mob uses (the flying speed for its
	// state); the host works it out.
	AirSpeed float32
	// Speed is the mob's speed setting (MovementSpeedComponent).
	Speed float32
	// MoveSpeed is the speed the move control last chose (Mob::mSpeed).
	MoveSpeed float32
	// LocalMove is the move input: strafe, up and forward.
	LocalMove Vec3
	// MaxAutoStep is the step height without jumping.
	MaxAutoStep  float32
	FallDistance float32

	// Jump is the jump control, nil when the mob has none.
	Jump *Jump
	// Look is the look control, nil when the mob has none.
	Look *Look

	// Vehicle is what the mob rides, nil when it rides nothing.
	Vehicle *Body
	// Target is where the mob's target stands, nil without a target.
	Target *Vec3
	// PathEnd is the last node of the mob's navigation path, nil without one.
	PathEnd *path.Pos
	// Eye is the mob's eye position.
	Eye Vec3
}

// SetSpeed is Mob::setSpeed: the speed setting and the forward input.
func (b *Body) SetSpeed(v float32) {
	b.Speed = v
	b.LocalMove.Z = v
}

// faceYaw turns both the body and the mob to yaw.
func (b *Body) faceYaw(yaw float32) {
	b.BodyYaw = yaw
	b.Yaw = yaw
}

const degrees = 57.295776

func atan2(y, x float32) float32 { return float32(math.Atan2(float64(y), float64(x))) }
func sqrt(v float32) float32     { return float32(math.Sqrt(float64(v))) }
func fmod(x, y float32) float32  { return float32(math.Mod(float64(x), float64(y))) }

// approach turns from towards to, clamped to max degrees.
func approach(from, to, max float32) float32 {
	d := fmod(to-from+180, 360)
	if d < 0 {
		d += 360
	}
	return max32(-max, min(max, d-180)) + from
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// boxCentre is the middle of a box.
func boxCentre(b Box) Vec3 {
	return Vec3{X: (b.Max.X-b.Min.X)*0.5 + b.Min.X, Y: (b.Max.Y-b.Min.Y)*0.5 + b.Min.Y, Z: (b.Max.Z-b.Min.Z)*0.5 + b.Min.Z}
}

// avgSize is the average of a box's three sizes.
func avgSize(b Box) float32 {
	return ((b.Max.Y - b.Min.Y) + ((b.Max.X - b.Min.X) + (b.Max.Z - b.Min.Z))) / 3
}

// turnYaw is the yaw a walking mob turns to for a move of dx, dz: it turns
// faster while it speeds up to newSpeed, and not at all for tiny moves.
func turnYaw(b *Body, maxTurn, dx, dz, newSpeed float32) float32 {
	w := ((b.Box.Max.Z - b.Box.Min.Z) + (b.Box.Max.X - b.Box.Min.X)) * 0.5 * 0.25
	r := max32(w, 0.05)
	if w > 0.25 {
		r = 0.25
	}
	if dz*dz+(dx*dx+0) <= r*r {
		return b.Yaw
	}
	target := atan2(dz, dx)*degrees - 90
	cur := b.MoveSpeed
	turn := maxTurn
	if newSpeed > cur && newSpeed > 0 && cur > 0 {
		t := maxTurn / (cur / newSpeed)
		turn = max32(t, maxTurn)
		if 2*maxTurn < t {
			turn = 2 * maxTurn
		}
	}
	return approach(b.Yaw, target, turn)
}

// moveSpeed is the speed a mob walks at towards p: it slows down over the
// last block of its path and speeds up by a tenth of speed per tick. Without
// a path it keeps its speed.
func moveSpeed(b *Body, p Vec3, speed float32) float32 {
	if b.PathEnd == nil {
		return b.MoveSpeed
	}
	var d2 float32
	if b.Flags.Has(FlagSwimming) || !b.OnGround {
		c := boxCentre(b.Box)
		d2 = (c.Z-p.Z)*(c.Z-p.Z) + ((c.X-p.X)*(c.X-p.X) + (c.Y-p.Y)*(c.Y-p.Y))
	} else {
		d2 = (b.Pos.Z-p.Z)*(b.Pos.Z-p.Z) + ((b.Pos.X-p.X)*(b.Pos.X-p.X) + (b.Pos.Y-p.Y)*(b.Pos.Y-p.Y))
	}
	cur := b.MoveSpeed
	end := *b.PathEnd
	var s float32
	if floor(p.X) == float32(end.X) && floor(p.Y) == float32(end.Y) && d2 < 1 && floor(p.Z+0) == float32(end.Z) {
		s = max32(0.5*speed, speed-(1-d2)*speed)
	} else {
		s = speed
		if speed > cur {
			s = min(speed, 0.1*speed+cur)
		}
	}
	if s > 0.25 && b.Pos.Y > p.Y {
		attr := b.MovementSpeed
		v := float32(0.25)
		if cur > attr {
			v = max32(attr, cur+speed*-0.1)
		}
		s = min(0.25, v)
	}
	b.MoveSpeed = s
	return s
}

func floor(v float32) float32 { return float32(math.Floor(float64(v))) }

// damp scales the mob's velocity.
func damp(b *Body, f float32) {
	b.Vel.X *= f
	b.Vel.Y *= f
	b.Vel.Z *= f
}
