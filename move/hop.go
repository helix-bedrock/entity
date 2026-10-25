package move

// JumpInfo is one kind of jump of the dynamic jump control: how far and
// high it goes and how long the mob waits before the next one.
type JumpInfo struct {
	DistanceScale float32
	Height        float32
	Delay         int32
	Animation     int8
}

// The dynamic jump control's jump types.
const (
	JumpNone   = 0
	JumpWalk   = 1
	JumpTake   = 2 // the jump being made
	JumpSprint = 3
)

// Dynamic is the jump.dynamic control used by hopping mobs. Infos holds its
// jump kinds by type; they come from the mob's definition.
type Dynamic struct {
	Type  int32
	Infos [4]JumpInfo
	// NoAnimation skips the jump animation flag.
	NoAnimation bool
}

func (d *Dynamic) info() JumpInfo {
	if d.Type < 0 || int(d.Type) >= len(d.Infos) {
		return JumpInfo{}
	}
	return d.Infos[d.Type]
}

// Hopper is the state minecraft:movement.jump (the hop control) keeps.
type Hopper struct {
	// Delay is the ticks left before the mob may hop again.
	Delay int32
	// IsJumping is the mob's is-jumping flag; JumpTicks its jump timer.
	IsJumping bool
	JumpTicks int32
	// SpeedModifier is the navigation speed modifier the jump sets.
	SpeedModifier float32
	// Animation is the jump animation the last jump asked for.
	Animation int8
}

// doJump starts a jump of the current type. Like BDS, it sets the speed
// modifier of both navigation and the move control.
func (h *Hopper) doJump(d *Dynamic, c *Control) {
	in := d.info()
	h.SpeedModifier = in.DistanceScale * 1.5
	c.Speed = h.SpeedModifier
	if !d.NoAnimation {
		h.Animation = in.Animation
	}
}

// TickJump is the dynamic jump control's tick: a requested jump becomes a
// jump of the take-off type.
func TickJump(b *Body, c *Control, d *Dynamic, h *Hopper) {
	if b.Jump != nil && b.Jump.Jumping {
		d.Type = JumpTake
		h.doJump(d, c)
		h.IsJumping = true
	}
	if b.Jump != nil {
		b.Jump.Jumping = false
	}
}

// Hop is minecraft:movement.jump: the mob moves in hops along its path,
// facing each node as it takes off.
//
// pathActive reports whether navigation has nodes left, and next is the
// centre of its current node.
func Hop(b *Body, c *Control, d *Dynamic, h *Hopper, pathActive bool, next Vec3) {
	if b.Jump == nil || d == nil {
		return
	}
	if c.Speed > 0.8 {
		d.Type = JumpSprint
	} else {
		d.Type = JumpWalk
	}
	if h.Delay > 0 {
		h.Delay--
	}
	if b.OnGround {
		if !b.WasOnGround {
			h.IsJumping = false
			h.Delay = d.info().Delay
			d.Type = JumpNone
		}
		if pathActive && h.Delay == 0 {
			b.Yaw = atan2(next.Z-b.Pos.Z, next.X-b.Pos.X)*degrees - 90
			h.JumpTicks = 0
			h.IsJumping = true
			h.doJump(d, c)
		}
	}
	if b.OnGround && !h.IsJumping {
		c.Speed = 0
		h.SpeedModifier = 0
	}
	if b.Flags.Has(FlagHasGravity) {
		b.LocalMove.Z = 0
	}
	if !c.HasWanted {
		return
	}
	dx, dz := c.Wanted.X-b.Pos.X, c.Wanted.Z-b.Pos.Z
	dy := c.Wanted.Y - b.Box.Min.Y
	d2 := (dx*dx + dy*dy) + dz*dz
	if b.Flags.Has(FlagCharging) || !b.Flags.Has(FlagHasGravity) {
		fly3D(b, c, dx, dy, dz, d2)
		return
	}
	if d2 < 0.1 {
		c.HasWanted = false
		return
	}
	w := ((b.Box.Max.Z - b.Box.Min.Z) + (b.Box.Max.X - b.Box.Min.X)) * 0.5 * 0.25
	r := max32(w, 0.05)
	if w > 0.25 {
		r = 0.25
	}
	if dz*dz+(dx*dx+0) > r*r {
		b.Yaw = approach(b.Yaw, atan2(dz, dx)*degrees-90, c.MaxTurn)
	}
	b.SetSpeed(b.MovementSpeed * c.Speed)
	if dy > 0.009 && max32(b.Width, 1) > dx*dx+dz*dz {
		b.Jump.Jumping = true
		if !h.IsJumping {
			h.IsJumping = true
			h.JumpTicks = 0
		}
	}
}

// SlimeJump is a slime's jump delay range in seconds.
type SlimeJump struct {
	Min, Max float32
}

// Slimer is the state the slime move control keeps.
type Slimer struct {
	Delay  int32
	inited bool
}

// slimeDelay is a random delay in ticks from the range.
func slimeDelay(j *SlimeJump, rnd func() float32) int32 {
	if j == nil {
		return 0
	}
	return int32(((j.Max-j.Min)*rnd() + j.Min) * 20)
}

// Slime is the slime move control: the mob faces its look target and jumps
// towards the wanted position at random intervals. rnd is the level's
// random float.
func Slime(b *Body, c *Control, s *Slimer, j *SlimeJump, rnd func() float32) {
	if b.Jump == nil {
		return
	}
	if !s.inited {
		s.Delay = slimeDelay(j, rnd)
		s.inited = true
	}
	yaw := b.Yaw
	if l := b.Look; l != nil {
		switch {
		case l.HasYaw:
			yaw = approach(yaw, l.Yaw, c.MaxTurn)
		case l.Has:
			yaw = approach(yaw, atan2(l.Pos.X-b.Pos.X, l.Pos.Z-b.Pos.Z)*-degrees, c.MaxTurn)
		}
	}
	b.HeadYaw = yaw
	b.BodyYaw = yaw
	b.Yaw = yaw
	if !c.HasWanted {
		b.LocalMove.Z = 0
		return
	}
	c.HasWanted = false
	b.SetSpeed(c.Speed * b.MovementSpeed)
	if !b.OnGround {
		return
	}
	old := s.Delay
	s.Delay--
	if old > 1 {
		b.SetSpeed(0)
		return
	}
	s.Delay = slimeDelay(j, rnd)
	s.inited = true
	b.Jump.Jumping = true
}
