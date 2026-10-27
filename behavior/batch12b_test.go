package behavior

import "testing"

// fakePlayer is fakeMob as a player.
type fakePlayer struct{ *fakeMob }

func (p *fakePlayer) IsPlayer() bool { return true }

// b12Rider adds the controlled_by_player and skeleton_horse_trap hooks to
// b12Mob.
type b12Rider struct {
	*b12Mob
	riders    []ID
	held      Item
	yaw       float32
	yaws      map[ID]float32
	vel       Vec3
	ground    bool
	jumps     int
	sprint    []bool
	removedMe int
	spawned   []string
	rides     map[ID]ID
	heads     int
	tamed     int
}

func newB12Rider(w World, p Vec3) *b12Rider {
	return &b12Rider{b12Mob: newB12Mob(w, p), yaws: map[ID]float32{}, rides: map[ID]ID{}}
}

func (m *b12Rider) Riders() []ID { return m.riders }
func (m *b12Rider) ControlItems() ([]string, bool) {
	return []string{"minecraft:carrot_on_a_stick"}, true
}
func (m *b12Rider) HeldItem(ID) Item                    { return m.held }
func (m *b12Rider) Yaw() float32                        { return m.yaw }
func (m *b12Rider) YawOf(id ID) float32                 { return m.yaws[id] }
func (m *b12Rider) SetYaw(y float32)                    { m.yaw = y }
func (m *b12Rider) SetSprinting(on bool)                { m.sprint = append(m.sprint, on) }
func (m *b12Rider) SetSpeed(float32)                    {}
func (m *b12Rider) ResetMovement()                      {}
func (m *b12Rider) Speed() float32                      { return 0.25 }
func (m *b12Rider) RideFriction(Pos) float32            { return 0.6 }
func (m *b12Rider) MaxAutoStep() (float32, bool)        { return 0.5625, true }
func (m *b12Rider) WalkThrough(Pos) bool                { return false }
func (m *b12Rider) Jump()                               { m.jumps++ }
func (m *b12Rider) MoveRelativeSpeed() float32          { return 0.1 }
func (m *b12Rider) OnGround() bool                      { return m.ground }
func (m *b12Rider) Velocity() Vec3                      { return m.vel }
func (m *b12Rider) SetVelocity(v Vec3)                  { m.vel = v }
func (m *b12Rider) Remove()                             { m.removedMe++ }
func (m *b12Rider) SpecialMultiplier() float32          { return 0 }
func (m *b12Rider) LightningRod(Pos) (Pos, bool)        { return Pos{}, false }
func (m *b12Rider) SpawnLightning(Vec3)                 { m.spawned = append(m.spawned, "lightning") }
func (m *b12Rider) SetInvulnerableTime(ID, int32)       {}
func (m *b12Rider) Push(ID, Vec3)                       {}
func (m *b12Rider) TameTo(ID, ID)                       { m.tamed++ }
func (m *b12Rider) HeadEmpty(ID) bool                   { return true }
func (m *b12Rider) GiveEnchantedHead(ID, string, int32) { m.heads++ }
func (m *b12Rider) EnchantHand(ID, int32)               {}
func (m *b12Rider) Ride(rider, mount ID)                { m.rides[rider] = mount }
func (m *b12Rider) Spawn(kind, _ string, p Vec3) (ID, bool) {
	m.spawned = append(m.spawned, kind)
	id := ID(100 + len(m.spawned))
	w := m.w.(*b11World)
	w.entities = append(w.entities, newMob(w, id, p, kind))
	return id, true
}

func TestControlledByPlayerNeedsTheControlItem(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Rider(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	p := &fakePlayer{newMob(w, 9, Vec3{X: 0.5, Y: 65, Z: 0.5}, "minecraft:player")}
	w.entities = append(w.entities, p)
	m.riders = []ID{p.id}
	g := NewControlledByPlayer(w, m, DefaultControlledByPlayer())
	if g.CanUse() {
		t.Fatal("steered without the control item")
	}
	m.held = Item{Name: "minecraft:carrot_on_a_stick", Count: 1}
	if !g.CanUse() {
		t.Fatal("not steered with the control item")
	}
}

func TestControlledByPlayerTurnsPartWayAndMoves(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Rider(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	p := &fakePlayer{newMob(w, 9, Vec3{X: 0.5, Y: 65, Z: 0.5}, "minecraft:player")}
	w.entities = append(w.entities, p)
	m.riders = []ID{p.id}
	m.yaws[p.id] = 90
	g := NewControlledByPlayer(w, m, DefaultControlledByPlayer())
	g.Tick()
	// Half of 90 degrees, capped at the 5 degree limit.
	if m.yaw != 5 {
		t.Fatalf("yaw %v, want 5", m.yaw)
	}
	if m.vel.Z <= 0 {
		t.Fatalf("no forward move: %v", m.vel)
	}
	g.Stop()
	if len(m.sprint) != 1 || m.sprint[0] {
		t.Fatal("stop did not end sprinting")
	}
}

func TestSkeletonHorseTrapSpringsOnAPlayer(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Rider(w, Vec3{X: 0.5, Y: 64, Z: 0.5})
	m.diff = difficultyHard
	p := &fakePlayer{newMob(w, 9, Vec3{X: 3.5, Y: 64, Z: 0.5}, "minecraft:player")}
	w.players = append(w.players, p)
	c := DefaultSkeletonHorseTrap()
	c.WithinRadius = 10
	g := NewSkeletonHorseTrap(w, m, c)
	if !g.CanUse() {
		t.Fatal("trap did not see the player")
	}
	g.Tick()
	// One bolt, then a horse and a rider four times.
	if len(m.spawned) != 9 || m.spawned[0] != "lightning" || len(m.rides) != 4 || m.heads != 4 || m.tamed != 4 {
		t.Fatalf("spawned %v, rides %d, heads %d", m.spawned, len(m.rides), m.heads)
	}
	if m.removedMe != 1 {
		t.Fatal("trap horse not removed")
	}
}

func TestSkeletonHorseTrapRunsOut(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB12Rider(w, Vec3{})
	g := NewSkeletonHorseTrap(w, m, DefaultSkeletonHorseTrap())
	for range ticksPerSecond + 1 {
		g.CanUse()
	}
	if m.removedMe != 1 {
		t.Fatalf("removed %d times", m.removedMe)
	}
}
