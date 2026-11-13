package behavior

import (
	"encoding/json"
	"testing"
)

// panicMob is a fakeMob that remembers who hurt it.
type panicMob struct {
	*fakeMob
	hurtBy   ID
	hurtTick uint64
}

func (m *panicMob) LastHurtBy() (ID, uint64) { return m.hurtBy, m.hurtTick }
func (m *panicMob) ClearLastHurtBy()         { m.hurtBy = NoID }
func (m *panicMob) LastDamageCause() int     { return -1 }
func (m *panicMob) PlaySound(int)            {}

func TestPanicContinuesWhileHurt(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}, tick: 100}
	m := &panicMob{fakeMob: newMob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cow"), hurtBy: 2, hurtTick: 90}
	zombie := newMob(w, 2, Vec3{X: 3.5, Y: 64, Z: 0.5}, "minecraft:zombie")
	w.entities = []Actor{m, zombie}
	g := NewPanic(w, m, PanicConfig{SpeedMultiplier: 1.25, Sound: NoSound})
	m.nav.done = true
	if !g.CanContinueToUse() {
		t.Fatal("panic stopped with its path done while the hurt still counts")
	}
	w.tick = 131
	if g.CanContinueToUse() {
		t.Fatal("panic kept going 41 ticks after the hurt with no path")
	}
}

func TestPanicRepicksWhenPathDone(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}, tick: 100}
	m := &panicMob{fakeMob: newMob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cow"), hurtBy: NoID}
	g := NewPanic(w, m, PanicConfig{SpeedMultiplier: 1.25, Sound: NoSound})
	old := Vec3{X: 100, Y: 64, Z: 100}
	g.pos = old
	m.nav.done = true
	g.Tick()
	if m.nav.moves != 1 || m.nav.to == old {
		t.Fatalf("tick moved %d times to %v, want one move to a new spot", m.nav.moves, m.nav.to)
	}
	m.nav.done = false
	g.pos = old
	w.tick += 9
	g.Tick()
	if m.nav.to != old {
		t.Fatalf("tick moved to %v while a path was running, want the old spot %v", m.nav.to, old)
	}
}

func TestRangedAttackJSONFormats(t *testing.T) {
	for _, c := range []struct {
		name, json             string
		lo, hi                 int32
		radiusMin, radius      float32
		inRange, sightTicks    int32
		shots, shootTrigger    int32
		persistent, followMode bool
	}{
		{name: "1.26.60 objects", json: `{"attack_interval":{"min":1,"max":3},"attack_range":{"min":0,"max":15},"set_persistent":true}`,
			lo: 20, hi: 60, radius: 15, sightTicks: 20, shots: 1, persistent: true},
		{name: "old fields", json: `{"attack_interval_min":1,"attack_interval_max":3,"attack_radius":15,"attack_radius_min":2}`,
			lo: 20, hi: 60, radiusMin: 2, radius: 15, sightTicks: 20, shots: 1},
		{name: "one interval number", json: `{"attack_interval":2.5,"attack_radius":64}`, lo: 50, hi: 50, radius: 64, sightTicks: 20, shots: 1},
		{name: "max below min", json: `{"attack_interval":{"min":3,"max":1}}`, lo: 60, hi: 60, sightTicks: 20, shots: 1},
		{name: "only min", json: `{"attack_interval_min":0.5}`, lo: 10, hi: 10, sightTicks: 20, shots: 1},
		{name: "times and mode", json: `{"target_in_sight_time":0.5,"burst_shots":3,"charge_shoot_trigger":1.25,"in_range_movement_mode":"follow_target"}`,
			sightTicks: 10, shots: 3, shootTrigger: 25, followMode: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := DefaultRangedAttack()
			if err := json.Unmarshal([]byte(c.json), &got); err != nil {
				t.Fatal(err)
			}
			mode := int32(0)
			if c.followMode {
				mode = rangedWalkIn
			}
			if got.AttackIntervalMin != c.lo || got.AttackIntervalMax != c.hi || got.AttackRadiusMin != c.radiusMin ||
				got.AttackRadius != c.radius || got.TargetInSightTime != c.sightTicks || got.BurstShots != c.shots ||
				got.ChargeShootTrigger != c.shootTrigger || got.SetPersistent != c.persistent || got.InRangeMovement != mode {
				t.Fatalf("got %+v", got)
			}
			if got.RangedFOV != 90 || got.SpeedMultiplier != 1 {
				t.Fatalf("defaults lost: %+v", got)
			}
		})
	}
}

func TestRangedAttackJSONErrors(t *testing.T) {
	for _, s := range []string{`{"in_range_movement_mode":"run_away"}`, `{"attack_interval":"fast"}`, `[]`} {
		c := DefaultRangedAttack()
		if err := json.Unmarshal([]byte(s), &c); err == nil {
			t.Errorf("%s: no error", s)
		}
	}
}

// v60Mob is a fakeMob with the pieces the 1.26.60 batch goals need.
type v60Mob struct {
	*fakeMob
	vel          Vec3
	speed        float32
	wanted       bool
	wantedAt     Vec3
	hasControl   bool
	caravan      bool
	head, tail   ID
	leader       ID
	alive        bool
	removedState bool
	leashed      bool
}

func newV60Mob(w World, id ID, p Vec3) *v60Mob {
	return &v60Mob{fakeMob: newMob(w, id, p, "minecraft:llama"), speed: 0.2, hasControl: true, head: NoID, tail: NoID, leader: NoID, alive: true}
}

func (m *v60Mob) Velocity() Vec3     { return m.vel }
func (m *v60Mob) SetVelocity(v Vec3) { m.vel = v }
func (m *v60Mob) Speed() float32     { return m.speed }
func (m *v60Mob) Alive() bool        { return m.alive }
func (m *v60Mob) Removed() bool      { return m.removedState }
func (m *v60Mob) MoveWanted() (bool, bool) {
	return m.wanted, m.hasControl
}
func (m *v60Mob) SetMoveWanted(p Vec3, _ float32) { m.wanted, m.wantedAt = true, p }
func (m *v60Mob) ClearMoveWanted()                { m.wanted = false }
func (m *v60Mob) Flag(n int) bool {
	if n == FlagLeashed {
		return m.leashed
	}
	return m.fakeMob.Flag(n)
}
func (m *v60Mob) InCaravan() bool       { return m.caravan }
func (m *v60Mob) CaravanHead() ID       { return m.head }
func (m *v60Mob) CaravanTail() ID       { return m.tail }
func (m *v60Mob) JoinCaravan(head ID)   { m.caravan, m.head = true, head }
func (m *v60Mob) LeaveCaravan()         { m.caravan, m.head = false, NoID }
func (m *v60Mob) TargetLeader() ID      { return m.leader }
func (m *v60Mob) SetTargetLeader(id ID) { m.leader = id }

func TestHoverHoldsStartSpot(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newV60Mob(w, 1, Vec3{X: 3, Y: 70, Z: 4})
	g := NewHover(w, m, DefaultHover())
	if g.CanUse() {
		t.Fatal("hover started on a mob that cannot fly")
	}
	m.flags[FlagCanFly] = true
	if !g.CanUse() {
		t.Fatal("hover did not start")
	}
	g.Start()
	m.pos = Vec3{X: 5, Y: 71, Z: 4}
	g.Tick()
	if !m.flags[FlagIdling] || !m.wanted || m.wantedAt != (Vec3{X: 3, Y: 70, Z: 4}) {
		t.Fatalf("idling %v wanted %v at %v, want the start spot", m.flags[FlagIdling], m.wanted, m.wantedAt)
	}
}

func TestFloatTemptPushesTowardPlayer(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newV60Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.speed = 0.5
	p := newMob(w, 2, Vec3{X: 10.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, p}
	c := TemptConfig{SpeedMultiplier: 2, Sound: NoSound}
	g := NewFloatTempt(w, m, c)
	g.player = 2
	g.Tick()
	if m.vel.X <= 0 || m.nav.moves != 0 {
		t.Fatalf("velocity %v, path moves %d, want a push along x and no path", m.vel, m.nav.moves)
	}
}

func TestFollowCaravanJoinsLeashedLeader(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newV60Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	lead := newV60Mob(w, 2, Vec3{X: 5.5, Y: 64, Z: 0.5})
	lead.leashed = true
	w.entities = []Actor{m, lead}
	g := NewFollowCaravan(w, m, DefaultFollowCaravan())
	if !g.CanUse() || m.head != 2 {
		t.Fatalf("did not join the leashed llama: head %v", m.head)
	}
	g.Start()
	g.Tick()
	if m.nav.to.X != 3.5 {
		t.Fatalf("walked to %v, want 2 blocks short of the leader", m.nav.to)
	}
}

func TestFollowTargetLeaderFindsLeader(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}, tick: 10}
	m := newV60Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5})
	l := newV60Mob(w, 2, Vec3{X: 6.5, Y: 64, Z: 0.5})
	w.entities = []Actor{m, l}
	c := DefaultFollowTargetLeader()
	c.WithinRadius, c.FollowDistance = 16, 2
	g := NewFollowTargetLeader(w, m, c)
	if !g.CanUse() || m.leader != 2 {
		t.Fatalf("leader %v, want 2", m.leader)
	}
}

func (m *v60Mob) LookBusy() bool                   { return true }
func (m *v60Mob) Riders() []ID                     { return nil }
func (m *v60Mob) HeadRotation() (float32, float32) { return 0, 0 }
