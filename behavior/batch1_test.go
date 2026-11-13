package behavior

import "testing"

// bodyMob adds the body state the newer goals read to fakeMob.
type bodyMob struct {
	*fakeMob
	ground bool
	vel    Vec3
	target ID
	owner  ID
	riders []ID
	hijack bool
	sit    bool
}

func (m *bodyMob) OnGround() bool                { return m.ground }
func (m *bodyMob) Velocity() Vec3                { return m.vel }
func (m *bodyMob) SetVelocity(v Vec3)            { m.vel = v }
func (m *bodyMob) Target() ID                    { return m.target }
func (m *bodyMob) SetTarget(id ID)               { m.target = id }
func (m *bodyMob) Owner() ID                     { return m.owner }
func (m *bodyMob) Riders() []ID                  { return m.riders }
func (m *bodyMob) SetHijackable(on bool)         { m.hijack = on }
func (m *bodyMob) SetPersistSit(on bool)         { m.sit = on }
func (m *bodyMob) SetPersistent()                {}
func (m *bodyMob) LookAt(Vec3, float32, float32) {}
func (m *bodyMob) LookBusy() bool                { return false }

func (m *bodyMob) HijacksMount() bool { return false }

func newBody(w World, id ID, p Vec3) *bodyMob {
	return &bodyMob{fakeMob: newMob(w, id, p, "minecraft:spider"), target: NoID, owner: NoID}
}

func TestTimerFlagRunsAndCoolsDown(t *testing.T) {
	w := &flatWorld{r: &seqRandom{ints: []int32{3, 5}}, tick: 10}
	m := newBody(w, 1, Vec3{})
	g := NewTimerFlag(w, m, FlagTimer1, TimerFlagConfig{Duration: Range[int32]{20, 40}, Cooldown: Range[int32]{50, 60}, OnEnd: "end"})
	if !g.CanUse() {
		t.Fatal("timer did not start")
	}
	g.Start()
	if !m.Flag(FlagTimer1) || g.until != 10+23 {
		t.Fatalf("start: flag %v until %d, want 33", m.Flag(FlagTimer1), g.until)
	}
	w.tick = 34
	if g.CanContinueToUse() {
		t.Fatal("timer ran past its duration")
	}
	g.Stop()
	if m.Flag(FlagTimer1) || g.ready != 34+55 || len(m.events) != 1 {
		t.Fatalf("stop: flag %v ready %d events %v", m.Flag(FlagTimer1), g.ready, m.events)
	}
}

func TestRandomRangeSkipsDrawForSpanOfOne(t *testing.T) {
	r := &seqRandom{ints: []int32{7}}
	if got := randomRange(r, 4, 5); got != 4 || len(r.ints) != 1 {
		t.Fatalf("randomRange(4, 5) = %d with %d draws left, want 4 and no draw", got, len(r.ints))
	}
}

func TestLeapPushesTowardTarget(t *testing.T) {
	w := &flatWorld{r: &seqRandom{ints: []int32{0}}}
	m := newBody(w, 1, Vec3{X: 0, Y: 64, Z: 0})
	m.ground = true
	prey := newMob(w, 2, Vec3{X: 3, Y: 64, Z: 0}, "minecraft:player")
	w.entities = []Actor{m, prey}
	m.target = 2
	g := NewLeap(w, m, LeapConfig{YD: 0.4, MustBeOnGround: true})
	if !g.CanUse() {
		t.Fatal("leap did not start")
	}
	g.Start()
	if m.vel.X != 0.4 || m.vel.Y != 0.4 || m.vel.Z != 0 {
		t.Fatalf("leap velocity %+v, want x 0.4 y 0.4", m.vel)
	}
}

func TestLeapNeedsTwoToFourBlocks(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newBody(w, 1, Vec3{Y: 64})
	m.ground = true
	prey := newMob(w, 2, Vec3{X: 1, Y: 64}, "minecraft:player")
	w.entities = []Actor{m, prey}
	m.target = 2
	if NewLeap(w, m, DefaultLeap()).CanUse() {
		t.Fatal("leapt at a target 1 block away")
	}
}

func TestOwnerHurtSkipsCreepers(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	pet := &petMob{bodyMob: newBody(w, 1, Vec3{Y: 64})}
	pet.flags[FlagTamed] = true
	pet.owner = 2
	owner := &ownerActor{fakeMob: newMob(w, 2, Vec3{X: 2, Y: 64}, "minecraft:player"), hurt: 3, stamp: 7}
	creeper := newMob(w, 3, Vec3{X: 4, Y: 64}, creeperType)
	w.entities = []Actor{pet, owner, creeper}
	g := NewOwnerHurt(w, pet)
	if g.CanUse() {
		t.Fatal("the pet targeted a creeper")
	}
	creeper.kind = "minecraft:zombie"
	if !g.CanUse() {
		t.Fatal("the pet did not target what its owner hit")
	}
	g.Start()
	if pet.target != 3 {
		t.Fatalf("target %d, want 3", pet.target)
	}
	pet.target = NoID
	if g.CanUse() {
		t.Fatal("the same hit was used twice")
	}
}

// petMob is a Targeter for the owner goals.
type petMob struct{ *bodyMob }

func (m *petMob) RuntimeID() uint64       { return 1 }
func (m *petMob) CanSee(Actor) bool       { return true }
func (m *petMob) CanAttack(Actor) bool    { return true }
func (m *petMob) FollowRange() float32    { return 16 }
func (m *petMob) Admiring() bool          { return false }
func (m *petMob) PlayersTargetable() bool { return true }
func (m *petMob) Team() int               { return 0 }

// ownerActor is a player who hit something.
type ownerActor struct {
	*fakeMob
	hurt  ID
	stamp int32
}

func (o *ownerActor) IsPlayer() bool           { return true }
func (o *ownerActor) LastHurtMob() (ID, int32) { return o.hurt, o.stamp }
func (o *ownerActor) LastHurtByMob() ID        { return NoID }

func TestMountPathingSteersMount(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	mount := newBody(w, 1, Vec3{Y: 64})
	rider := newBody(w, 2, Vec3{Y: 65})
	prey := newMob(w, 3, Vec3{X: 10, Y: 64}, "minecraft:player")
	w.entities = []Actor{mount, rider, prey}
	mount.riders = []ID{2}
	rider.target = 3
	g := NewMountPathing(w, mount, MountPathingConfig{SpeedMultiplier: 1.5, TrackTarget: true})
	if !g.CanUse() {
		t.Fatal("mount_pathing did not start")
	}
	g.Start()
	if !mount.hijack || mount.nav.speed != 1.5 {
		t.Fatalf("hijackable %v speed %v", mount.hijack, mount.nav.speed)
	}
	g.Stop()
	if mount.hijack {
		t.Fatal("stop left the mount hijackable")
	}
}

func TestBoxOverlapIsStrict(t *testing.T) {
	a := Box{Max: Vec3{X: 1, Y: 1, Z: 1}}
	b := Box{Min: Vec3{X: 1}, Max: Vec3{X: 2, Y: 1, Z: 1}}
	if overlaps(a, b) {
		t.Fatal("touching boxes overlap")
	}
	b.Min.X = 0.9
	if !overlaps(a, b) {
		t.Fatal("overlapping boxes do not overlap")
	}
}
