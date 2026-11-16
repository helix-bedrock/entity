package behavior

import "math"

// The melee goal's fixed numbers.
const (
	meleeReuseChance = 0.05 // the chance to repath although the path still fits
	meleeMinWidth    = 0.5
	meleeRideWidth   = 1.39 // the widest vehicle that widens the reach
	meleeRidePad     = 0.1
	meleeBoxReach    = 0.8 // melee_box_attack's default horizontal_reach
	// meleeFirstCooldown is the attack timer a melee goal starts with: BDS's
	// base constructor sets it from its default cooldown before
	// cooldown_time is applied.
	meleeFirstCooldown = 20
)

// Attacker is the part of the mob melee_attack needs on top of Targeter.
type Attacker interface {
	Targeter
	// HasAttack reports the minecraft:attack component (the actor
	// definition's attack description); melee goals do not start without it.
	HasAttack() bool
	// Slot is the item in a hand slot; SlotMainhand is Mob::getCarriedItem.
	Slot(slot int) Item
	// Difficulty is the level's difficulty, 0 (peaceful) to 3 (hard).
	Difficulty() int
	// Ignite sets the entity with the ID on fire for at least seconds
	// (Actor::setOnFire). It is called only from AttackResult.
	Ignite(id ID, seconds int32)
	// Swing swings the mob's arm.
	Swing()
	// Attack hits the target and, when then is not nil, tells then whether it hit and
	// whether the target died. An engine that ticks mobs one at a time calls then before
	// Attack returns; one whose mobs think in parallel applies the hit after the thinking
	// and calls then there, still in the same tick and before the mob's next one.
	Attack(a Actor, then AttackResult)
}

// AttackResult receives the outcome of an Attack.
type AttackResult interface {
	AttackResult(target ID, hit, killed bool)
}

// MeleeConfig is the JSON of minecraft:behavior.melee_attack. Times are in
// ticks.
type MeleeConfig struct {
	SpeedMultiplier     float32 `json:"speed_multiplier"`
	TrackTarget         bool    `json:"track_target"`
	RequireCompletePath bool    `json:"require_complete_path"`
	AttackOnce          bool    `json:"attack_once"`
	ReachMultiplier     float32 `json:"reach_multiplier"`
	MeleeFOV            float32 `json:"melee_fov"`
	Cooldown            int32
	RandomStopInterval  int32 `json:"random_stop_interval"`
	PathTime            Range[int32]
	InnerBoundary       float32 `json:"path_inner_boundary"`
	OuterBoundary       float32 `json:"path_outer_boundary"`
	InnerIncrease       int32
	OuterIncrease       int32
	FailIncrease        int32
	XMaxRotation        float32 `json:"x_max_rotation"`
	YMaxHeadRotation    float32 `json:"y_max_head_rotation"`
	// HorizontalReach is melee_box_attack's reach: how far the mob's box
	// is widened, on x and z only, to touch the target's box.
	HorizontalReach float32 `json:"horizontal_reach"`
	// AttackTypes is attack_types as an actor category mask
	// (ActorCategories); when it is not 0 the target must be in every
	// category of it.
	AttackTypes uint32
	// CanSpreadOnFire is can_spread_on_fire: a hit with an empty hand sets
	// the target alight while the mob burns.
	CanSpreadOnFire bool
	OnAttack        string
	OnKill          string
	// FireChance is the chance by difficulty (peaceful to hard) that a
	// burning mob's hit spreads its fire; the target burns for twice its
	// whole part in seconds. Above hard the chance is FireChanceAbove.
	FireChance      [4]float32
	FireChanceAbove float32
}

// DefaultMelee returns the goal's BDS defaults (MeleeAttackGoal's constructor).
func DefaultMelee() MeleeConfig {
	return MeleeConfig{
		SpeedMultiplier: 1, ReachMultiplier: 2, MeleeFOV: 90, Cooldown: 20,
		PathTime: Range[int32]{4, 11}, InnerBoundary: 16, OuterBoundary: 32,
		InnerIncrease: 5, OuterIncrease: 10, FailIncrease: 15, XMaxRotation: 30, YMaxHeadRotation: 30,
		FireChance: [4]float32{0, 0.42, 1.14, 1.92}, FireChanceAbove: -0.3,
	}
}

// DefaultMeleeBox returns melee_box_attack's BDS defaults: melee_attack's, with a
// horizontal reach of 0.8.
func DefaultMeleeBox() MeleeConfig {
	c := DefaultMelee()
	c.HorizontalReach = meleeBoxReach
	return c
}

// Melee is MeleeAttackGoal: the mob chases its target and hits it in reach.
type Melee struct {
	base
	m        Attacker
	cfg      MeleeConfig
	target   ID
	cooldown int32
	repath   int32
	done     bool
	// last is where the chase step last saw the target (MeleeAttackBaseGoal
	// +136); delayed_attack and stomp_attack measure their reach from it.
	last Vec3
	// box is set for MeleeBoxAttackGoal, which only replaces the tick.
	box bool
}

// NewMelee makes the goal. As BDS's constructor does, the first attack
// waits meleeFirstCooldown ticks.
func NewMelee(w World, m Attacker, c MeleeConfig) *Melee {
	return &Melee{base: base{mob: m, world: w, flags: flagsMoveLook}, m: m, cfg: c, target: NoID, cooldown: meleeFirstCooldown}
}

// NewMeleeBox makes MeleeBoxAttackGoal: melee_attack whose reach is the
// mob's box widened by horizontal_reach touching the target's box, and whose
// hits need a clear line through blocks.
func NewMeleeBox(w World, m Attacker, c MeleeConfig) *Melee {
	g := NewMelee(w, m, c)
	g.box = true
	return g
}

// CanUse implements goal.Goal (BDS 1.26.60 MeleeAttackBaseGoal 0x54325B0).
// A mob that rides passes without a path. A sun-avoiding navigation skips a
// target standing in the sun while the mob is out of it (0x5432A70).
// set_persistent is never set on this goal in 1.26.60 (0x5604EF0), and
// hijack_mount_navigation is not built: no vanilla mob uses it.
func (g *Melee) CanUse() bool {
	if g.m.Admiring() {
		return g.no("admiring an item")
	}
	n := g.m.Nav()
	if n == nil {
		return g.no("no navigation")
	}
	if !g.m.HasAttack() {
		return g.no("no attack component")
	}
	t := g.actor(g.m.Target())
	if t == nil || !t.Alive() {
		return g.no("no living target")
	}
	if g.cfg.AttackTypes != 0 && ^ActorCategoriesOf(t)&g.cfg.AttackTypes != 0 {
		return g.no("target not in attack_types")
	}
	g.target = t.ID()
	if g.m.Vehicle() != NoID {
		return true
	}
	if n.Flags()&navAvoidSun != 0 && g.sunBlocked(t) {
		return g.no("target in the sun")
	}
	ok, complete := n.PathToStatus(t)
	if !ok {
		return g.no("no path to target")
	}
	if g.cfg.RequireCompletePath && !complete {
		return g.no("path does not reach target (require_complete_path)")
	}
	return true
}

// sunBlocked is 0x5432A70: the mob is out of the sun and the target, not
// removed, is in it.
func (g *Melee) sunBlocked(t Actor) bool {
	return !t.Removed() && !sunlit(g.world, g.m.Pos()) && sunlit(g.world, t.Pos())
}

// sunlit is the sun test of 0x5432A70 at an entity's feet: daytime, and the
// block at or above the build limit or at or above the height map.
func sunlit(w World, p Vec3) bool {
	if !w.IsDay() {
		return false
	}
	b := blockOf(p)
	if b.Y >= w.MaxHeight() {
		return true
	}
	return b.Y >= w.MinHeight() && b.Y >= w.HeightMap(b.X, b.Z)
}

// CanContinueToUse implements goal.Goal (BDS 1.26.60 MeleeAttackBaseGoal
// 0x5432EF0). attack_once after a hit, the random stop (thread random) and
// a monster target in peaceful clear the mob's target as they stop.
func (g *Melee) CanContinueToUse() bool {
	if g.m.Admiring() {
		return false
	}
	if g.done {
		g.m.SetTarget(NoID)
		return false
	}
	if g.cfg.RandomStopInterval > 0 && g.world.ThreadRandom().NextInt(g.cfg.RandomStopInterval) == 0 {
		g.m.SetTarget(NoID)
		return false
	}
	t := g.actor(g.m.Target())
	if t == nil || t.ID() != g.target || !t.Alive() {
		return false
	}
	if g.world.Peaceful() && t.Monster() {
		g.m.SetTarget(NoID)
		return false
	}
	if g.m.Vehicle() != NoID || g.cfg.TrackTarget {
		return true
	}
	n := g.m.Nav()
	return n != nil && !n.Done()
}

// Start implements goal.Goal. BDS follows the path canUse made, none for a
// riding mob, and sets the repath timer to path_fail_time_increase
// (0x5433220).
func (g *Melee) Start() {
	g.done = false
	g.repath = g.cfg.FailIncrease
	n := g.m.Nav()
	if n == nil {
		return
	}
	if t := g.actor(g.target); t != nil && g.m.Vehicle() == NoID {
		n.MoveToActor(t, g.cfg.SpeedMultiplier)
	} else {
		n.Stop()
	}
}

// Stop implements goal.Goal.
func (g *Melee) Stop() {
	g.target = NoID
	if n := g.m.Nav(); n != nil {
		n.Stop()
	}
}

// reach2 is the squared attack reach of 2.4.
func (g *Melee) reach2(t Actor) float32 {
	b := g.m.Box()
	w := max32(meleeMinWidth, b.Max.X-b.Min.X)
	w2 := w
	if v := g.actor(g.m.Vehicle()); v != nil {
		vb := v.Box()
		w2 = max32(min(vb.Max.X-vb.Min.X, meleeRideWidth), w) + meleeRidePad
	}
	r := g.cfg.ReachMultiplier*w - w + w2
	tb := t.Box()
	return r*r + (tb.Max.X - tb.Min.X)
}

// inFOV is the melee_fov test.
func (g *Melee) inFOV(t Actor) bool { return inFOV(g.m, t.Pos(), g.cfg.MeleeFOV) }

// inFOV is the field of view test (0x7267140): q lies within fov degrees
// of the mob's body yaw.
func inFOV(m Mob, q Vec3, fov float32) bool {
	p := m.Pos()
	yaw := float32(math.Atan2(float64(q.Z-p.Z), float64(q.X-p.X)))*degPerRad - 90 - m.BodyYaw()
	return wrap180(yaw+180) <= fov*0.5
}

// Tick implements goal.Goal.
func (g *Melee) Tick() {
	t := g.actor(g.target)
	if t == nil || ridesOn(g.world, t, g.m.ID()) {
		return
	}
	if g.box {
		g.boxTick(t)
		return
	}
	g.cooldown--
	if !g.m.LookBusy() {
		g.m.LookAt(lookPoint(t), g.cfg.YMaxHeadRotation, g.cfg.XMaxRotation)
	}
	g.repath--
	seen := g.m.CanSee(t)
	d2 := dist2(g.m.Pos(), t.Pos())
	if (g.cfg.TrackTarget || seen) && g.repath <= 0 {
		g.chase(t, d2, d2 <= g.reach2(t))
	}
	if !seen || d2 > g.reach2(t) || !g.inFOV(t) || g.cooldown > 0 {
		return
	}
	g.strike(t)
}

// strike is the attack step of 2.4 once the target is in reach and seen.
func (g *Melee) strike(t Actor) {
	g.cooldown = g.cfg.Cooldown
	g.m.Swing()
	g.m.Attack(t, g)
}

// AttackResult implements AttackResult: the end of 2.4's attack step, the events and
// attack_once, once the engine knows whether the hit landed.
func (g *Melee) AttackResult(target ID, hit, killed bool) {
	if !hit {
		return
	}
	g.spreadFire(target)
	if g.cfg.OnAttack != "" {
		g.m.Event(g.cfg.OnAttack, target)
	}
	if killed && g.cfg.OnKill != "" {
		g.m.Event(g.cfg.OnKill, target)
	}
	if g.cfg.AttackOnce {
		g.done = true
	}
}

// spreadFire is can_spread_on_fire (BDS 1.26.60 _attemptAttackTarget
// 0x5431880): a burning mob with an empty hand sets the target alight by a
// difficulty chance drawn on the level's IRandom.
func (g *Melee) spreadFire(target ID) {
	if !g.cfg.CanSpreadOnFire || !g.m.OnFire() {
		return
	}
	c := g.cfg.FireChanceAbove
	if d := g.m.Difficulty(); d >= 0 && d < len(g.cfg.FireChance) {
		c = g.cfg.FireChance[d]
	}
	if c > g.world.IRandom().NextFloat() && g.m.Slot(SlotMainhand).Empty() {
		g.m.Ignite(target, 2*int32(c))
	}
}

// step is the chase step the melee goals share (1.26.60 0x5431120): look
// at the target (the look control's yaw limit is y_max_head_rotation, its
// pitch limit x_max_rotation), count the repath timer down, and while the target is
// tracked or seen remember where it stands (its feet, the bottom of its
// box) and repath when the timer runs out.
func (g *Melee) step(t Actor, inReach bool, d2 float32) {
	if !g.m.LookBusy() {
		g.m.LookAt(lookPoint(t), g.cfg.YMaxHeadRotation, g.cfg.XMaxRotation)
	}
	g.repath--
	if !g.cfg.TrackTarget && !g.m.CanSee(t) {
		return
	}
	g.last = Vec3{X: t.Pos().X, Y: t.Box().Min.Y, Z: t.Pos().Z}
	if g.repath <= 0 {
		g.chase(t, d2, inReach)
	}
}

// keepPath is the repath skip: a path already ending at the remembered
// position is kept 95 times in 100 (thread random); another path is kept
// while the target is in reach. No path, or no remembered position, never
// keeps.
func (g *Melee) keepPath(n Navigator, inReach bool) bool {
	if g.last == (Vec3{}) {
		return false
	}
	if n != nil {
		end, ok := n.PathEnd()
		if !ok {
			return false
		}
		if end != blockOf(g.last) {
			return inReach
		}
	}
	return g.world.ThreadRandom().NextFloat() >= meleeReuseChance
}

// chase is the repath step: the timer is drawn again from path_time (thread
// random), lengthened by distance and by a failed path.
func (g *Melee) chase(t Actor, d2 float32, inReach bool) {
	n := g.m.Nav()
	if g.keepPath(n, inReach) {
		return
	}
	r := g.world.ThreadRandom()
	g.repath = randomInt(r, g.cfg.PathTime.Min, g.cfg.PathTime.Max)
	switch {
	case d2 > g.cfg.OuterBoundary*g.cfg.OuterBoundary:
		g.repath += g.cfg.OuterIncrease
	case d2 > g.cfg.InnerBoundary*g.cfg.InnerBoundary:
		g.repath += g.cfg.InnerIncrease
	}
	if n == nil || !n.MoveToActor(t, g.cfg.SpeedMultiplier) {
		g.repath += g.cfg.FailIncrease
	}
}

// boxTick is MeleeBoxAttackGoal's tick (1.26.60 0x5433900): the reach is
// a box overlap with the cooldown spent, the path distance is flat to the
// target's vehicle (one level, as BDS takes it) or the target, and a hit
// needs a block-free line between the eyes.
func (g *Melee) boxTick(t Actor) {
	b := g.m.Box()
	r := g.cfg.HorizontalReach
	reach := Box{
		Min: Vec3{X: min(b.Min.X-r, b.Max.X+r), Y: min(b.Min.Y, b.Max.Y), Z: min(b.Min.Z-r, b.Max.Z+r)},
		Max: Vec3{X: max(b.Min.X-r, b.Max.X+r), Y: max(b.Min.Y, b.Max.Y), Z: max(b.Min.Z-r, b.Max.Z+r)},
	}
	ready := true
	if g.cooldown > 0 {
		g.cooldown--
		ready = g.cooldown == 0
	}
	tb := t.Box()
	inReach := ready && overlaps(reach, tb)
	v := g.actor(t.Vehicle())
	if v == nil {
		v = t
	}
	p, q := g.m.Pos(), v.Pos()
	d2 := (q.X-p.X)*(q.X-p.X) + (q.Z-p.Z)*(q.Z-p.Z)
	clear := g.world.ClearLine(g.m.Eyes(), t.Eyes())
	g.step(t, inReach, d2)
	if inReach && clear && g.inFOV(t) {
		g.strike(t)
	}
}

// overlaps is the strict AABB intersection test.
func overlaps(a, b Box) bool {
	return a.Max.X > b.Min.X && b.Max.X > a.Min.X && a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y && a.Min.Z < b.Max.Z && b.Min.Z < a.Max.Z
}

// rootVehicle is the lowest entity of a's riding stack, or a itself.
func rootVehicle(w World, a Actor) Actor {
	for i := 0; i < maxRideDepth; i++ {
		v := w.Actor(a.Vehicle())
		if v == nil {
			return a
		}
		a = v
	}
	return a
}

// ridesOn reports that a rides the entity with the ID, directly or further
// down its stack.
func ridesOn(w World, a Actor, id ID) bool {
	for i := 0; i < maxRideDepth; i++ {
		v := w.Actor(a.Vehicle())
		if v == nil {
			return false
		}
		if v.ID() == id {
			return true
		}
		a = v
	}
	return false
}
