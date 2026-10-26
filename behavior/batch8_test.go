package behavior

import "testing"

// b8World adds chests, beds and thunder to b7World.
type b8World struct {
	*b7World
	open    map[Pos]bool
	head    map[Pos]int32
	thunder float32
	players []Actor
}

func (w *b8World) ChestOpen(p Pos) bool        { return w.open[p] }
func (w *b8World) ThunderLevel() float32       { return w.thunder }
func (w *b8World) Players(dst []Actor) []Actor { return append(dst, w.players...) }
func (w *b8World) BlockState(p Pos, s string) (int32, bool) {
	if s == stateHeadPiece {
		v, ok := w.head[p]
		return v, ok
	}
	return w.b7World.BlockState(p, s)
}

func newB8World(r *seqRandom) *b8World {
	return &b8World{b7World: newB7World(r), open: map[Pos]bool{}, head: map[Pos]int32{}}
}

// b8Mob adds damage, health, effects, held items and the sneeze hooks.
type b8Mob struct {
	*b7Mob
	stamp    uint64
	cause    int
	amount   float32
	health   int32
	effects  []int32
	carried  map[ID]string
	jumped   []ID
	levelEv  []int32
	dropped  []string
	teleport []Vec3
}

func newB8Mob(w World, id ID, p Vec3, kind string) *b8Mob {
	return &b8Mob{b7Mob: newB7Mob(w, id, p, kind), cause: -1, health: 14, carried: map[ID]string{}}
}

func (m *b8Mob) LastDamage() (uint64, int, float32) { return m.stamp, m.cause, m.amount }
func (m *b8Mob) Health() int32                      { return m.health }
func (m *b8Mob) AddEffect(id int, ticks, _ int32)   { m.effects = append(m.effects, int32(id), ticks) }
func (m *b8Mob) Carried(a Actor) (string, bool) {
	it, ok := m.carried[a.ID()]
	return it, ok
}
func (m *b8Mob) MakeJump(a Actor)                         { m.jumped = append(m.jumped, a.ID()) }
func (m *b8Mob) LevelEventData(id int32, _ Vec3, _ int32) { m.levelEv = append(m.levelEv, id) }
func (m *b8Mob) DropLoot(t string)                        { m.dropped = append(m.dropped, t) }
func (m *b8Mob) Teleport(p Vec3)                          { m.teleport = append(m.teleport, p); m.pos = p }

func TestLayDownStartsAndStopsByChance(t *testing.T) {
	w := newB8World(&seqRandom{ints: []int32{3, 0, 5, 0}})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:panda")
	g := NewLayDown(w, m, DefaultLayDown)
	if g.CanUse() || !g.CanUse() {
		t.Fatal("lay_down should start on a 0 draw only")
	}
	g.Start()
	if !m.Flag(FlagLayingDown) || !g.CanContinueToUse() || g.CanContinueToUse() {
		t.Fatal("lay_down should stop on a 0 draw only")
	}
	m.wet[1] = true
	if g.CanUse() {
		t.Fatal("lay_down started in water")
	}
}

func TestScaredInAThunderstorm(t *testing.T) {
	w := newB8World(&seqRandom{ints: []int32{0}})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:panda")
	g := NewScared(w, m, ScaredConfig{SoundInterval: 5})
	w.thunder = 0.9
	if g.CanUse() {
		t.Fatal("scared at the 0.9 edge")
	}
	w.thunder = 1
	m.flags[FlagTransitionSitting] = true
	m.flags[FlagSitting] = false
	if !g.CanUse() {
		t.Fatal("not scared in a full storm")
	}
	g.Start()
	g.Tick()
	if !m.Flag(FlagScared) || len(m.played) != 1 || m.played[0] != scaredSound {
		t.Fatalf("start: flag %v sounds %v", m.Flag(FlagScared), m.played)
	}
	g.Stop()
	if m.Flag(FlagScared) || !m.Flag(FlagSitting) || !m.Flag(FlagTransitionSitting) {
		t.Fatal("stop did not leave the panda sitting")
	}
}

func TestSneezeStartlesAndDrops(t *testing.T) {
	w := newB8World(&seqRandom{floats: []float32{0.01, 0.5}})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:panda")
	o := newB8Mob(w, 2, Vec3{X: 2, Y: 64}, "minecraft:panda")
	w.entities = []Actor{m, o}
	c := DefaultSneeze
	c.Cooldown, c.Sound, c.PrepareSound, c.LootTable, c.WithinRadius = 2, 7, 8, "loot/panda", 4
	c.EntityTypes = []Filter{func(_ Mob, a Actor) bool { return a.Type() == "minecraft:panda" }}
	g := NewSneeze(w, m, c)
	for i := 0; i < 2; i++ {
		if g.CanUse() {
			t.Fatal("sneeze ignored its cooldown")
		}
	}
	if !g.CanUse() {
		t.Fatal("sneeze did not start after its cooldown")
	}
	g.Start()
	if !m.Flag(FlagSneezing) || g.timer != 20 || m.played[0] != 8 {
		t.Fatalf("start: flag %v timer %d sounds %v", m.Flag(FlagSneezing), g.timer, m.played)
	}
	for g.CanContinueToUse() {
		g.Tick()
	}
	if len(m.levelEv) != 1 || len(m.jumped) != 1 || m.jumped[0] != 2 || len(m.dropped) != 1 {
		t.Fatalf("sneeze: events %v jumped %v dropped %v", m.levelEv, m.jumped, m.dropped)
	}
	g.Stop()
	if m.Flag(FlagSneezing) || g.cooldown != 2 {
		t.Fatal("stop did not reset the cooldown")
	}
}

func TestNapWakesForMobs(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:fox")
	o := newB8Mob(w, 2, Vec3{X: 3, Y: 64}, "minecraft:chicken")
	w.entities = []Actor{m}
	c := DefaultNap
	c.WakeMobExceptions = func(_ Mob, a Actor) bool { return a.Type() == "minecraft:fox" }
	g := NewNap(w, m, c)
	if !g.CanUse() {
		t.Fatal("did not nap with no one around")
	}
	g.Start()
	if !m.Flag(FlagSleeping) || g.detect != 20 {
		t.Fatalf("start: flag %v detect %d", m.Flag(FlagSleeping), g.detect)
	}
	w.entities = []Actor{m, o}
	if !g.CanContinueToUse() {
		t.Fatal("woke before the next look")
	}
	w.tick = 21
	if g.CanContinueToUse() {
		t.Fatal("slept through a chicken")
	}
	g.Stop()
	if m.Flag(FlagSleeping) || g.next != 21 {
		t.Fatal("stop kept sleeping or set a cooldown")
	}
}

func TestFindCoverPicksACoveredSpot(t *testing.T) {
	w := newB8World(&seqRandom{ints: []int32{10, 3, 10}})
	m := newB8Mob(w, 1, Vec3{X: 0.5, Y: 70, Z: 0.5}, "minecraft:fox")
	g := NewFindCover(w, m, FindCoverConfig{SpeedMultiplier: 1, Cooldown: 100})
	if g.CanUse() {
		t.Fatal("found cover where every spot is open or not wanted")
	}
}

func TestPlayDeadOncePerHit(t *testing.T) {
	w := newB8World(&seqRandom{floats: []float32{0.1}, ints: []int32{1}})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:axolotl")
	c := DefaultPlayDead
	c.Duration, c.RandomStartChance, c.ForceBelowHealth = 200, 0.33, 8
	c.RandomDamageRange = Range[int32]{Min: 0, Max: 2}
	c.ApplyRegeneration = true
	g := NewPlayDead(w, m, c)
	if g.CanUse() {
		t.Fatal("played dead with no damage")
	}
	m.stamp, m.cause, m.amount = 1, 2, 3
	if !g.CanUse() {
		t.Fatal("did not play dead after a big hit")
	}
	if g.CanUse() {
		t.Fatal("played dead twice for one hit")
	}
	g.Start()
	if !m.Flag(FlagPlayingDead) || len(m.effects) != 2 || m.effects[0] != effectRegeneration || g.until != 200 {
		t.Fatalf("start: flag %v effects %v until %d", m.Flag(FlagPlayingDead), m.effects, g.until)
	}
	m.stamp, m.health = 2, 5
	if !g.CanUse() {
		t.Fatal("did not play dead below force_below_health")
	}
	g.Stop()
	if m.Flag(FlagPlayingDead) {
		t.Fatal("stop kept the flag")
	}
}

func TestBegStaresAtFood(t *testing.T) {
	w := newB8World(&seqRandom{ints: []int32{20}})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:wolf")
	p := newB8Mob(w, 2, Vec3{X: 3, Y: 64}, "minecraft:player")
	p.player = true
	w.entities = []Actor{m, p}
	w.players = []Actor{p}
	g := NewBeg(w, m, BegConfig{Items: []string{"minecraft:bone"}, LookDistance: 8, LookTime: Range[int32]{Min: 40, Max: 80}})
	if g.CanUse() {
		t.Fatal("begged at an empty hand")
	}
	m.carried[2] = "minecraft:bone"
	if !g.CanUse() {
		t.Fatal("did not beg at a bone")
	}
	g.Start()
	g.Tick()
	if !m.Flag(FlagInterested) || g.timer != 59 || m.looked != 1 {
		t.Fatalf("beg: flag %v timer %d looks %d", m.Flag(FlagInterested), g.timer, m.looked)
	}
	p.pos.X = 9
	if g.CanContinueToUse() {
		t.Fatal("kept begging out of range")
	}
	g.Stop()
	if m.Flag(FlagInterested) || g.player != NoID {
		t.Fatal("stop kept the player")
	}
}

func TestOcelotSitOnBlockValidBlocks(t *testing.T) {
	w := newB8World(&seqRandom{})
	m := newB8Mob(w, 1, Vec3{Y: 64}, "minecraft:cat")
	g := NewOcelotSitOnBlock(w, m, DefaultMoveTo)
	chest, bed := Pos{X: 1, Y: 64}, Pos{X: 2, Y: 64}
	w.blocks[chest] = Block{Name: blockChest, ShapeTop: 0.875}
	w.blocks[bed] = Block{Name: blockBed}
	if !g.valid(chest) {
		t.Fatal("a closed chest was not valid")
	}
	w.open[chest] = true
	if g.valid(chest) {
		t.Fatal("an open chest was valid")
	}
	w.head[bed] = 1
	if g.valid(bed) {
		t.Fatal("a bed's head was valid")
	}
	w.head[bed] = 0
	if !g.valid(bed) {
		t.Fatal("a bed's foot was not valid")
	}
	g.reachable(chest)
	if g.offset.Y != 0.875 {
		t.Fatalf("offset %v, want the chest's shape top", g.offset)
	}
	m.flags[FlagTamed] = true
	g.target = bed
	m.pos = Vec3{X: 2, Y: 63.1, Z: 0}
	g.offset = Vec3{}
	g.Tick()
	if !m.Flag(FlagSitting) || len(m.teleport) != 1 || m.teleport[0] != (Vec3{X: 2.5, Y: 65, Z: 0.5}) {
		t.Fatalf("tick at the bed: sitting %v moved %v", m.Flag(FlagSitting), m.teleport)
	}
}
