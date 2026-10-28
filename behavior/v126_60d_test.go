package behavior

import "testing"

// knightMob is a b2Mob holding a kinetic weapon.
type knightMob struct {
	*b2Mob
	weapon   bool
	charging bool
	starts   []float32
	stops    int
}

func (m *knightMob) Nav() Navigator { return pathNav{m.nav} }
func (m *knightMob) Swing()         {}
func (m *knightMob) Attack(Actor) (bool, bool) {
	return true, false
}
func (m *knightMob) KineticReach() (Range[float32], bool) { return Range[float32]{1, 3}, m.weapon }
func (m *knightMob) StartKineticCharge(s float32) {
	m.starts = append(m.starts, s)
	m.charging = true
}
func (m *knightMob) KineticCharging() bool { return m.charging }
func (m *knightMob) StopKineticCharge()    { m.stops++; m.charging = false }

func newKineticTest() (*knightMob, *b2Mob, *UseKineticWeapon, *flatWorld) {
	w := &flatWorld{r: &seqRandom{}}
	m := &knightMob{b2Mob: newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:zombie"), weapon: true}
	t := newB2Mob(w, 2, Vec3{X: 20.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, t}
	m.target = 2
	c := DefaultUseKineticWeapon()
	c.WeaponMinSpeedMultiplier = 0.5
	return m, t, NewUseKineticWeapon(w, m, c), w
}

func TestUseKineticWeaponNeedsWeapon(t *testing.T) {
	m, _, g, _ := newKineticTest()
	if !g.CanUse() {
		t.Fatal("did not start with a kinetic weapon")
	}
	m.weapon = false
	if g.CanUse() {
		t.Fatal("started without a kinetic weapon")
	}
}

func TestUseKineticWeaponChargesAndCoolsDown(t *testing.T) {
	m, target, g, w := newKineticTest()
	if !g.CanUse() {
		t.Fatal("did not start")
	}
	g.Start()
	if g.repath != 15 || g.reach != (Range[float32]{1, 3}) {
		t.Fatalf("repath %d reach %v, want the fail increase and the weapon's reach", g.repath, g.reach)
	}
	// 20 blocks away, past approach_distance 8: still chasing.
	g.Tick()
	if g.state != kineticApproach || len(m.starts) != 0 {
		t.Fatalf("state %d starts %v, want approaching", g.state, m.starts)
	}
	m.pos = Vec3{X: 14.5, Y: 64, Z: 0.5}
	g.Tick()
	if g.state != kineticCharge || len(m.starts) != 1 || m.starts[0] != 0.5 {
		t.Fatalf("state %d starts %v, want a charge with the min speed multiplier", g.state, m.starts)
	}
	// Within the reach's middle (2): run past the target.
	m.pos = Vec3{X: 19.5, Y: 64, Z: 0.5}
	g.Tick()
	if g.state != kineticReposition || !g.hasSpot {
		t.Fatalf("state %d spot %v, want repositioning", g.state, g.hasSpot)
	}
	// The angle is turned from degrees twice, so the spot lands
	// reposition_distance on +z of the target whatever the direction.
	if g.spot != (Pos{X: 20, Y: 65, Z: 2}) {
		t.Fatalf("spot %v, want 2 blocks on +z of the target's eyes", g.spot)
	}
	// The attack ends: cool down for the melee cooldown.
	m.charging = false
	g.Tick()
	if g.state != kineticCooldown || m.stops != 1 || g.cooldownEnd != w.tick+20 {
		t.Fatalf("state %d stops %d end %d, want a 20 tick cooldown", g.state, m.stops, g.cooldownEnd)
	}
	m.nav.done = true
	g.Tick()
	if g.state != kineticCooldown {
		t.Fatalf("state %d, want the cooldown to last", g.state)
	}
	w.tick += 21
	g.Tick()
	if g.state != kineticApproach {
		t.Fatalf("state %d, want a new approach after the cooldown", g.state)
	}
	_ = target
	g.Stop()
	if m.stops != 2 {
		t.Fatalf("stops %d, want the charge cleared on stop", m.stops)
	}
}

// witherMob is a b2Mob with the wither's hooks.
type witherMob struct {
	*b2Mob
	wants, dashing bool
	air            float32
	delay          int32
	tally          []Damager
}

func (m *witherMob) WantsDash() bool         { return m.wants }
func (m *witherMob) StartDash()              { m.dashing = true }
func (m *witherMob) EndDash(d int32)         { m.wants, m.dashing, m.delay = false, false, d }
func (m *witherMob) ScaleAirSpeed(f float32) { m.air *= f }
func (m *witherMob) Damagers(dst []Damager) []Damager {
	return append(dst, m.tally...)
}

func TestWitherRandomAttackPosDashes(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := &witherMob{b2Mob: newB2Mob(w, 1, Vec3{X: 0.5, Y: 70, Z: 0.5}, "minecraft:wither"), air: 0.5}
	tg := newB2Mob(w, 2, Vec3{X: 5.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, tg}
	g := NewWitherRandomAttackPos(w, m, DefaultWitherRandomAttackPos())
	if g.CanUse() {
		t.Fatal("dashed without a target")
	}
	m.target = 2
	if g.CanUse() {
		t.Fatal("dashed without the wither asking")
	}
	m.wants = true
	m.flags[FlagPowered] = true
	if g.CanUse() {
		t.Fatal("dashed while powered")
	}
	m.flags[FlagPowered] = false
	if !g.CanUse() {
		t.Fatal("did not dash")
	}
	if g.wanted.Y != 70 || dist2(Vec3{X: g.wanted.X, Z: g.wanted.Z}, Vec3{X: 5.5, Z: 0.5}) > 100.01 {
		t.Fatalf("wanted %v, want xz_dist past the target at the mob's height", g.wanted)
	}
	g.Start()
	if !m.dashing || m.air != 7.5 || m.nav.to != g.wanted {
		t.Fatalf("dashing %v air %v to %v, want a fast walk to the spot", m.dashing, m.air, m.nav.to)
	}
	m.nav.done = true
	g.Tick()
	if g.CanContinueToUse() {
		t.Fatal("kept going after the path ended")
	}
	g.Stop()
	if m.air != 0.5 || m.wants || m.dashing || m.delay != 20 {
		t.Fatalf("air %v wants %v dashing %v delay %d, want the dash cleared", m.air, m.wants, m.dashing, m.delay)
	}
}

// calmWorld is a flatWorld that is not peaceful.
type calmWorld struct{ *flatWorld }

func (calmWorld) Peaceful() bool { return false }

func TestWitherTargetHighestDamage(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := &witherMob{b2Mob: newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:wither")}
	a := newB2Mob(w, 2, Vec3{X: 5.5, Y: 64, Z: 0.5}, "minecraft:player")
	b := newB2Mob(w, 3, Vec3{X: 6.5, Y: 64, Z: 0.5}, "minecraft:player")
	far := newB2Mob(w, 4, Vec3{X: 100.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, a, b, far}
	m.tally = []Damager{{ID: 2, Damage: 5}, {ID: 4, Damage: 50}, {ID: 3, Damage: 8}}
	g := NewWitherTargetHighestDamage(calmWorld{w}, m, DefaultWitherTargetHighestDamage())
	if !g.CanUse() || g.candidate != 3 {
		t.Fatalf("candidate %d, want the highest damager within the follow range", g.candidate)
	}
	g.Start()
	if m.target != 3 || !g.CanContinueToUse() {
		t.Fatalf("target %d, want 3 kept", m.target)
	}
	m.tally = append(m.tally, Damager{ID: 2, Damage: 20})
	if g.CanContinueToUse() {
		t.Fatal("kept the target after another dealt more damage")
	}
}
