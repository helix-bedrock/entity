package behavior

import (
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// hurt is one HurtBy call.
type hurt struct {
	to     ID
	cause  int
	damage float32
	ignore bool
}

// b2Mob is a v60Mob with the hooks of the second 1.26.60 batch.
type b2Mob struct {
	*v60Mob
	target, owner ID
	bound         *Pos
	chasing       map[ID]bool
	lookedAt      []Vec3
	hurts         []hurt
	entityEvents  []int
	summons       []Summon
	sent          []string
	sounds        []int
	difficulty    int
}

func newB2Mob(w World, id ID, p Vec3, kind string) *b2Mob {
	m := &b2Mob{v60Mob: newV60Mob(w, id, p), target: NoID, owner: NoID, chasing: map[ID]bool{}}
	m.kind = kind
	return m
}

func (m *b2Mob) Target() ID                             { return m.target }
func (m *b2Mob) SetTarget(id ID)                        { m.target = id }
func (m *b2Mob) Owner() ID                              { return m.owner }
func (m *b2Mob) Eyes() Vec3                             { return Vec3{X: m.pos.X, Y: m.pos.Y + 1.6, Z: m.pos.Z} }
func (m *b2Mob) LookAt(p Vec3, _, _ float32)            { m.lookedAt = append(m.lookedAt, p) }
func (m *b2Mob) MaxHeadXRot() float32                   { return 40 }
func (m *b2Mob) CanAttack(Actor) bool                   { return true }
func (m *b2Mob) IgnoresCannotBeAttacked(Actor) bool     { return false }
func (m *b2Mob) RuntimeID() uint64                      { return uint64(m.id) }
func (m *b2Mob) CanSee(Actor) bool                      { return true }
func (m *b2Mob) Admiring() bool                         { return false }
func (m *b2Mob) PlayersTargetable() bool                { return true }
func (m *b2Mob) BoundOrigin() (Pos, bool)               { return derefPos(m.bound) }
func (m *b2Mob) ChasingDuringPlay(id ID) bool           { return m.chasing[id] }
func (m *b2Mob) SetChasingDuringPlay(on bool)           { m.chasing[m.id] = on }
func (m *b2Mob) Difficulty() int                        { return m.difficulty }
func (m *b2Mob) AttackDamage() (float32, float32, bool) { return 4, 6, true }
func (m *b2Mob) HurtBy(a Actor, c int, d float32, i bool) {
	m.hurts = append(m.hurts, hurt{to: a.ID(), cause: c, damage: d, ignore: i})
}
func (m *b2Mob) EntityEvent(id int)               { m.entityEvents = append(m.entityEvents, id) }
func (m *b2Mob) ResetAttack()                     {}
func (m *b2Mob) CountEntities(string, Box) int    { return 0 }
func (m *b2Mob) Summon(s Summon)                  { m.summons = append(m.summons, s) }
func (m *b2Mob) SetSpellColor(int32)              {}
func (m *b2Mob) SendEvent(to ID, e string)        { m.sent = append(m.sent, e) }
func (m *b2Mob) PlaySoundAt(s int, _ Vec3)        { m.sounds = append(m.sounds, s) }
func (m *b2Mob) Team() int                        { return 0 }
func (m *b2Mob) Monster() bool                    { return false }
func (m *b2Mob) FollowRange() float32             { return 64 }
func (m *b2Mob) ArmourCoverage() float32          { return 0 }
func (m *b2Mob) Invulnerable() bool               { return false }
func (m *b2Mob) WearsHeadOf(string) bool          { return false }
func (m *b2Mob) HeadRotation() (float32, float32) { return 0, 0 }
func (m *b2Mob) LastHurtMob() (ID, int32)         { return NoID, 0 }
func (m *b2Mob) LastHurtByMob() ID                { return NoID }
func (m *b2Mob) Dimension() int                   { return 0 }
func (m *b2Mob) BlocksSpace() bool                { return false }
func (m *b2Mob) HijacksMount() bool               { return false }
func (m *b2Mob) BodyYaw() float32                 { return 0 }
func (m *b2Mob) AttachPos(int) Vec3               { return m.Eyes() }
func (m *b2Mob) HasHealth() bool                  { return true }

func derefPos(p *Pos) (Pos, bool) {
	if p == nil {
		return Pos{}, false
	}
	return *p, true
}

// lineWorld is a flatWorld with a ClearLine answer.
type lineWorld struct {
	*flatWorld
	blocked bool
}

func (w *lineWorld) ClearLine(Vec3, Vec3) bool { return !w.blocked }

// stoneWorld is a flatWorld whose stone has a solid material (the material
// number is arbitrary; only air is tested).
type stoneWorld struct{ *flatWorld }

func (w stoneWorld) Block(p Pos) Block {
	b := w.flatWorld.Block(p)
	if b.Solid {
		b.Material = path.MaterialLava + 1
	}
	return b
}

func TestVexCopyOwnerTarget(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	vex := newB2Mob(w, 1, Vec3{X: 0.5, Y: 70, Z: 0.5}, "minecraft:vex")
	evoker := newB2Mob(w, 2, Vec3{X: 3.5, Y: 64, Z: 0.5}, "minecraft:evocation_illager")
	player := newB2Mob(w, 3, Vec3{X: 8.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{vex, evoker, player}
	vex.owner = 2
	g := NewVexCopyOwnerTarget(w, vex)
	if g.CanUse() {
		t.Fatal("copied a target the owner does not have")
	}
	evoker.target = 3
	player.flags[FlagCannotTarget] = true
	if g.CanUse() {
		t.Fatal("copied a target with the cannot-target flag")
	}
	player.flags[FlagCannotTarget] = false
	if !g.CanUse() {
		t.Fatal("did not copy the owner's target")
	}
	g.Start()
	if vex.target != 3 {
		t.Fatalf("target %v, want 3", vex.target)
	}
}

func TestVexRandomMoveFindsAir(t *testing.T) {
	// Offsets 14-7, 10-5, 7-7 from the bound origin (0,60,0): the first spot
	// (7,65,0) is air.
	w := &flatWorld{r: &seqRandom{ints: []int32{0, 14, 10, 7}}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 70, Z: 0.5}, "minecraft:vex")
	m.bound = &Pos{Y: 60}
	g := NewVexRandomMove(w, m, DefaultVexRandomMove())
	if !g.CanUse() {
		t.Fatal("did not start on a 0 roll")
	}
	g.Tick()
	if want := (Vec3{X: 7.5, Y: 65.5, Z: 0.5}); !m.wanted || m.wantedAt != want {
		t.Fatalf("wanted %v at %v, want %v", m.wanted, m.wantedAt, want)
	}
	if len(m.lookedAt) != 1 {
		t.Fatalf("looked %d times, want once with no target", len(m.lookedAt))
	}
	if g.CanUse() {
		t.Fatal("started again with a wanted position")
	}
}

func TestVexRandomMoveSkipsSolid(t *testing.T) {
	// Every spot is at y 55 to 57: stone.
	w := stoneWorld{&flatWorld{r: &seqRandom{ints: []int32{0, 7, 0, 7, 7, 1, 7, 7, 2, 7}}}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 70, Z: 0.5}, "minecraft:vex")
	m.bound = &Pos{Y: 60}
	g := NewVexRandomMove(w, m, DefaultVexRandomMove())
	g.CanUse()
	g.Tick()
	if m.wanted {
		t.Fatalf("flew into stone at %v", m.wantedAt)
	}
}

func TestPlayChasesFriend(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:villager_v2")
	far := newB2Mob(w, 2, Vec3{X: 5.5, Y: 64, Z: 0.5}, "minecraft:villager_v2")
	near := newB2Mob(w, 3, Vec3{X: 3.5, Y: 64, Z: 0.5}, "minecraft:villager_v2")
	cow := newB2Mob(w, 4, Vec3{X: 1.5, Y: 64, Z: 0.5}, "minecraft:cow")
	w.entities = []Actor{m, far, near, cow}
	c := DefaultPlay()
	g := NewPlay(w, m, c)
	if g.CanUse() && g.friend != NoID {
		t.Fatal("picked a friend with no friend_types")
	}
	c.FriendTypes = []Filter{nil}
	g.SetConfig(c)
	if !g.CanUse() || g.friend != 3 {
		t.Fatalf("friend %v, want the nearest villager 3", g.friend)
	}
	g.Start()
	if !m.chasing[1] || g.timer != 1000 {
		t.Fatalf("chasing %v timer %d", m.chasing[1], g.timer)
	}
	g.Tick()
	if m.nav.to != near.pos {
		t.Fatalf("ran to %v, want the friend", m.nav.to)
	}
	g.Stop()
	if m.chasing[1] {
		t.Fatal("chase flag left on")
	}
}

func TestPlayChanceToStart(t *testing.T) {
	w := &flatWorld{r: &seqRandom{floats: []float32{0.5}}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:villager_v2")
	if NewPlay(w, m, DefaultPlay()).CanUse() {
		t.Fatal("started on a 0.5 roll")
	}
}

func TestGuardianBeam(t *testing.T) {
	for _, c := range []struct {
		name   string
		elder  bool
		hard   bool
		ticks  int
		magic  float32
		ignore bool
	}{
		{name: "normal", ticks: 90, magic: 1},
		{name: "hard", hard: true, ticks: 90, magic: 3},
		{name: "elder hard", elder: true, hard: true, ticks: 70, magic: 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			fw := &flatWorld{r: &seqRandom{floats: []float32{0.5}}}
			w := &lineWorld{flatWorld: fw}
			m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 60, Z: 0.5}, "minecraft:guardian")
			p := newB2Mob(w, 2, Vec3{X: 8.5, Y: 60, Z: 0.5}, "minecraft:player")
			fw.entities = []Actor{m, p}
			m.target = 2
			m.flags[FlagElder] = c.elder
			if c.hard {
				m.difficulty = difficultyHard
			}
			g := NewGuardianAttack(w, m, DefaultGuardianAttack())
			if !g.CanUse() {
				t.Fatal("did not start")
			}
			g.Start()
			for range c.ticks - 1 {
				g.Tick()
			}
			if len(m.hurts) != 0 {
				t.Fatalf("hit %d ticks early", 1)
			}
			g.Tick()
			if len(m.entityEvents) != 1 || m.entityEvents[0] != 28 {
				t.Fatalf("entity events %v, want the beam sound once", m.entityEvents)
			}
			if len(m.hurts) != 2 || m.hurts[0] != (hurt{to: 2, cause: 2, damage: 5}) ||
				m.hurts[1] != (hurt{to: 2, cause: 14, damage: c.magic, ignore: true}) {
				t.Fatalf("hurts %+v", m.hurts)
			}
			if m.target != NoID {
				t.Fatal("kept the target after the hit")
			}
		})
	}
}

func TestGuardianLosesSight(t *testing.T) {
	fw := &flatWorld{r: &seqRandom{}}
	w := &lineWorld{flatWorld: fw, blocked: true}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 60, Z: 0.5}, "minecraft:guardian")
	p := newB2Mob(w, 2, Vec3{X: 8.5, Y: 60, Z: 0.5}, "minecraft:player")
	fw.entities = []Actor{m, p}
	m.target = 2
	g := NewGuardianAttack(w, m, DefaultGuardianAttack())
	g.Start()
	g.Tick()
	if m.target != NoID {
		t.Fatal("kept a target behind a wall")
	}
}

func TestSummonLine(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:evocation_illager")
	p := newB2Mob(w, 2, Vec3{X: 10.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	ch := DefaultSummonChoice()
	ch.Weight = 1
	s := DefaultSummonSequence()
	s.Size, s.NumEntitiesSpawned, s.EntityType, s.EntityLifespan = 6, 3, "minecraft:evocation_fang", 22
	ch.Sequence = []SummonSequence{s}
	ch.CastDuration = ch.TotalDelay()
	c := DefaultSummonEntity()
	c.SummonChoices = []SummonChoice{ch}
	g := NewSummonEntity(w, m, c)
	if !g.CanUse() {
		t.Fatal("did not start")
	}
	g.Start()
	g.Tick()
	if len(m.summons) != 3 {
		t.Fatalf("summoned %d, want 3 in one tick", len(m.summons))
	}
	for i, want := range []float32{2, 4, 6} {
		if got := m.summons[i]; got.Pos != (Vec3{X: want, Y: 64, Z: 0}) || got.Lifespan != 22 {
			t.Errorf("summon %d at %v life %d, want x %v on the floor", i, got.Pos, got.Lifespan, want)
		}
	}
	if g.CanContinueToUse() {
		t.Fatal("kept going after the only sequence")
	}
	g.Stop()
	if g.CanUse() != true {
		t.Fatal("a 0 cooldown blocked the next cast")
	}
}

func TestSummonOutOfRange(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:evocation_illager")
	p := newB2Mob(w, 2, Vec3{X: 40.5, Y: 64, Z: 0.5}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.target = 2
	ch := DefaultSummonChoice()
	ch.Weight = 1
	ch.Sequence = []SummonSequence{DefaultSummonSequence()}
	c := DefaultSummonEntity()
	c.SummonChoices = []SummonChoice{ch}
	if NewSummonEntity(w, m, c).CanUse() {
		t.Fatal("cast past max_activation_range")
	}
}

func TestSendEventStages(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:evocation_illager")
	sheep := newB2Mob(w, 2, Vec3{X: 5.5, Y: 64, Z: 0.5}, "minecraft:sheep")
	w.entities = []Actor{m, sheep}
	ch := DefaultSendEventChoice()
	ch.Weight = 1
	ch.Sequence = []SendEventStage{{BaseDelay: 2, Event: "wololo", SoundEvent: NoSound}, {BaseDelay: 1, Event: "after", SoundEvent: 9}}
	ch.CastDuration = ch.TotalDelay()
	c := DefaultSendEvent()
	c.EventChoices = []SendEventChoice{ch}
	g := NewSendEvent(w, m, c)
	if !g.CanUse() || g.to != 2 {
		t.Fatalf("event target %v, want the sheep", g.to)
	}
	g.Start()
	if !m.flags[FlagCasting] {
		t.Fatal("not casting")
	}
	for range 3 {
		g.Tick()
	}
	if len(m.sent) != 1 || m.sent[0] != "wololo" || len(m.sounds) != 1 || m.sounds[0] != 9 {
		t.Fatalf("sent %v sounds %v after 3 ticks", m.sent, m.sounds)
	}
	g.Tick()
	if len(m.sent) != 2 || g.CanContinueToUse() {
		t.Fatalf("sent %v, continuing %v", m.sent, g.CanContinueToUse())
	}
	if m.flags[FlagCasting] {
		t.Fatal("still casting after cast_duration")
	}
}
