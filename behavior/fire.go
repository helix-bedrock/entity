package behavior

import "math"

// The fire_at_target goal's fixed numbers.
const (
	fireSound = 54 // the level sound played with each shot
	// The attach locations BDS maps owner_anchor and target_anchor to:
	// 1 is the eyes, 2 the body, anything else the feet.
	fireAnchorEyes = 1
	fireAnchorBody = 2
	attachFeet     = 0
	attachBody     = 1
	attachWeapon   = 2
	attachHead     = 3
	attachEyes     = 6
)

// Firer is the part of the mob fire_at_target needs on top of Targeter.
type Firer interface {
	Targeter
	MoveController
	// SpawnProjectile spawns the projectile definition from from, aimed
	// along dir, with the mob as its owner.
	SpawnProjectile(def string, from, dir Vec3)
}

// FireAtTargetConfig is the JSON of minecraft:behavior.fire_at_target.
// Times are in ticks.
type FireAtTargetConfig struct {
	// Filters is tested with the mob as the subject and no other entity.
	Filters          Filter
	AttackRange      Range[float32]
	AttackCooldown   int32
	PreShootDelay    int32
	PostShootDelay   int32
	MaxHeadRotationX float32 `json:"max_head_rotation_x"`
	MaxHeadRotationY float32 `json:"max_head_rotation_y"`
	RangedFOV        float32 `json:"ranged_fov"`
	ProjectileDef    string  `json:"projectile_def"`
	OwnerAnchor      int32   `json:"owner_anchor"`
	OwnerOffset      Vec3
	TargetAnchor     int32 `json:"target_anchor"`
	TargetOffset     Vec3
	// RotateOffset turns the offsets' x by the shooter's yaw. BDS works it
	// out from the experiments when it builds the goal (map?).
	RotateOffset bool
}

// DefaultFireAtTarget returns the goal's BDS defaults.
func DefaultFireAtTarget() FireAtTargetConfig {
	return FireAtTargetConfig{
		AttackCooldown: 10, PreShootDelay: 15, PostShootDelay: 4, MaxHeadRotationX: 30, MaxHeadRotationY: 30,
		RangedFOV: 90, OwnerAnchor: fireAnchorBody, TargetAnchor: fireAnchorBody, RotateOffset: true,
	}
}

// FireAtTarget is FireAtTargetGoal: the mob turns to its target and fires
// one projectile after a delay.
type FireAtTarget struct {
	base
	m        Firer
	cfg      FireAtTargetConfig
	fired    bool
	cooldown uint64
	shoot    uint64
	end      uint64
}

// NewFireAtTarget makes the goal.
func NewFireAtTarget(w World, m Firer, c FireAtTargetConfig) *FireAtTarget {
	return &FireAtTarget{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c}
}

// CanUse implements goal.Goal (0x573CD60).
func (g *FireAtTarget) CanUse() bool {
	if g.world.CurrentTick() < g.cooldown || g.cfg.ProjectileDef == "" {
		return false
	}
	if g.cfg.Filters != nil && !g.cfg.Filters(g.m, nil) {
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil || !t.Alive() {
		return false
	}
	d := dist2(t.AttachPos(attachFeet), g.m.AttachPos(attachFeet))
	lo, hi := g.cfg.AttackRange.Min, g.cfg.AttackRange.Max
	return d < hi*hi && lo*lo < d
}

// CanContinueToUse implements goal.Goal (0x573CEC0).
func (g *FireAtTarget) CanContinueToUse() bool {
	return g.actor(g.m.Target()) != nil && g.world.CurrentTick() <= g.end
}

// Start implements goal.Goal (0x573CF30).
func (g *FireAtTarget) Start() {
	now := g.world.CurrentTick()
	g.shoot = now + uint64(g.cfg.PreShootDelay)
	g.end = now + uint64(g.cfg.PreShootDelay+g.cfg.PostShootDelay)
	g.m.SetFlag(FlagRangedAttacking, true)
}

// Stop implements goal.Goal (0x573CF80).
func (g *FireAtTarget) Stop() {
	g.shoot, g.fired = 0, false
	g.cooldown = g.world.CurrentTick() + uint64(g.cfg.AttackCooldown)
	g.m.SetFlag(FlagRangedAttacking, false)
}

// Tick implements goal.Goal (0x573D0D0).
func (g *FireAtTarget) Tick() {
	t := g.actor(g.m.Target())
	if t == nil || t.Removed() || !t.Alive() {
		return
	}
	if turn, ok := g.m.MoveControl(); ok {
		p, q := g.m.Pos(), t.Pos()
		faceYaw(g.m, g.m, float32(math.Atan2(float64(q.X-p.X), float64(q.Z-p.Z)))*-degPerRad, turn)
	}
	g.m.LookAt(lookPoint(t), g.cfg.MaxHeadRotationY, g.cfg.MaxHeadRotationX)
	if g.fired || g.world.CurrentTick() < g.shoot {
		return
	}
	g.fired = true
	if !inViewCone(g.m, t.Pos(), g.cfg.RangedFOV) {
		return
	}
	to := anchorPos(t, g.cfg.TargetAnchor, g.cfg.TargetOffset, g.cfg.RotateOffset)
	from := anchorPos(g.m, g.cfg.OwnerAnchor, g.cfg.OwnerOffset, g.cfg.RotateOffset)
	g.m.SpawnProjectile(g.cfg.ProjectileDef, from, sub(to, from))
	g.m.PlaySoundAt(fireSound, g.m.AttachPos(attachWeapon))
}

// anchorPos is fire_at_target's anchor and offset point (0x573CFD0): the
// anchor's attach position plus the offset scaled by the box size. BDS
// only applies the x and y of the offset; z only feeds the rotation.
func anchorPos(a Actor, anchor int32, off Vec3, rotate bool) Vec3 {
	x := off.X
	if rotate {
		_, yaw := a.HeadRotation()
		r := float64(yaw * -radPerDeg)
		x = off.X*float32(math.Cos(r)) + off.Z*float32(math.Sin(r))
	}
	loc := attachFeet
	switch anchor {
	case fireAnchorEyes:
		loc = attachEyes
	case fireAnchorBody:
		loc = attachBody
	}
	b := a.Box()
	p := a.AttachPos(loc)
	return Vec3{X: (b.Max.X-b.Min.X)*x + p.X, Y: (b.Max.Y-b.Min.Y)*off.Y + p.Y, Z: p.Z}
}

// inViewCone is the shot's view test (0x7267140): the angle between the
// view vector from the eyes and the line to p is below fov degrees. BDS
// takes the previous pitch and the yaw at partial tick 0.
func inViewCone(m Mob, p Vec3, fov float32) bool {
	pitch, _ := m.PrevRotation()
	_, yaw := m.HeadRotation()
	a := (yaw*-radPerDeg - math.Pi) * sinScale
	c, s := tableSin(uint16(int32(a+sinQuarter))), tableSin(uint16(int32(a)))
	b := pitch * -radPerDeg * sinScale
	cp, sp := tableSin(uint16(int32(b+sinQuarter))), tableSin(uint16(int32(b)))
	d := sub(p, m.AttachPos(attachEyes))
	dir := normalize(d)
	dot := sp*dir.Y - s*cp*dir.X - c*cp*dir.Z
	return fov > float32(math.Acos(float64(dot)))*degPerRad
}
