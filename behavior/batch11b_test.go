package behavior

import "testing"

// b11bMob adds the eat, carry, treasure, swim and dig hooks to b11Mob.
type b11bMob struct {
	*b11Mob
	standing  Pos
	eatEvents int
	removed   []ID
	dropped   []string
	pulled    map[ID]Vec3
	carried   string
	seeBlock  bool
	wreck     Pos
	hasWreck  bool
	resets    int
	motion    map[ID]Vec3
	loot      int
	defaultOK bool
}

func newB11bMob(w World, p Vec3, kind string) *b11bMob {
	return &b11bMob{b11Mob: newB11Mob(w, p, kind), pulled: map[ID]Vec3{}, seeBlock: true, motion: map[ID]Vec3{}}
}

func (m *b11bMob) StandingOn() Pos                     { return m.standing }
func (m *b11bMob) EatGameEvent(Vec3)                   { m.eatEvents++ }
func (m *b11bMob) SetVelocityOf(id ID, v Vec3)         { m.pulled[id] = v }
func (m *b11bMob) DropLoot(table string, _ Vec3, _ ID) { m.dropped = append(m.dropped, table) }
func (m *b11bMob) RemoveActor(id ID)                   { m.removed = append(m.removed, id) }
func (m *b11bMob) BlockItem(b Block) Item              { return Item{Name: b.Name, Count: 1} }
func (m *b11bMob) CarriedBlock() (string, bool)        { return m.carried, m.carried != "" }
func (m *b11bMob) CanSeeBlock(Pos) bool                { return m.seeBlock }
func (m *b11bMob) NearestStructure(k string, _ Pos) (Pos, bool) {
	return m.wreck, m.hasWreck && k == structureShipwreck
}
func (m *b11bMob) LootChest(Box) (Pos, bool)    { return Pos{}, false }
func (m *b11bMob) FollowRange() float32         { return 16 }
func (m *b11bMob) ResetTreasureTimer()          { m.resets++ }
func (m *b11bMob) Motion(id ID) Vec3            { return m.motion[id] }
func (m *b11bMob) Rotation() (float32, float32) { return 0, 0 }
func (m *b11bMob) SpawnLoot(string, Vec3) bool  { m.loot++; return true }
func (m *b11bMob) DefaultDigBlock(b Block) bool { return m.defaultOK }
func (m *b11bMob) HasEffect(int) bool           { return false }

func TestEatBlockEatsGrassAfterTheTimer(t *testing.T) {
	w := newB11World(&seqRandom{floats: []float32{0.01}})
	w.griefing = true
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:sheep")
	w.blocks[Pos{Y: 63}] = Block{Name: "minecraft:grass_block"}
	m.standing = Pos{Y: 63}
	g := NewEatBlock(w, m, DefaultEatBlock())
	if !g.CanUse() {
		t.Fatal("a sheep on grass did not want to eat")
	}
	g.Start()
	for g.CanContinueToUse() {
		g.Tick()
	}
	if len(w.setTo) != 1 || w.setTo[0].Name != "minecraft:dirt" || m.eatEvents != 1 {
		t.Fatalf("eating set %v, %d eat events", w.setTo, m.eatEvents)
	}
}

func TestEatBlockDrawsTheChanceFirst(t *testing.T) {
	w := newB11World(&seqRandom{floats: []float32{0.5}})
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:sheep")
	w.blocks[Pos{Y: 63}] = Block{Name: "minecraft:grass_block"}
	if NewEatBlock(w, m, DefaultEatBlock()).CanUse() {
		t.Fatal("ate past a failed chance roll")
	}
}

func TestTakeBlockPicksUpAListedBlock(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{2, 1, 2}})
	w.griefing = true
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:enderman")
	w.blocks[Pos{X: 0, Y: 65, Z: 0}] = Block{Name: "minecraft:dirt"}
	c := DefaultTakeBlock()
	c.Blocks = []string{"minecraft:dirt"}
	g := NewTakeBlock(w, m, c)
	if !g.CanUse() {
		t.Fatal("an empty-handed enderman would not take a block")
	}
	g.Tick()
	if m.Slot(SlotMainhand).Name != "minecraft:dirt" || len(w.setTo) != 1 || w.setTo[0].Name != airName {
		t.Fatalf("took %v, set %v", m.Slot(SlotMainhand), w.setTo)
	}
	if g.CanUse() {
		t.Fatal("took a second block with full hands")
	}
}

func TestPlaceBlockPutsTheCarriedBlockDown(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{2, 0, 2}})
	w.griefing = true
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:enderman")
	m.carried = "minecraft:dirt"
	m.slots[SlotMainhand] = Item{Name: "minecraft:dirt", Count: 1}
	g := NewPlaceBlock(w, m, DefaultPlaceBlock())
	if !g.CanUse() {
		t.Fatal("would not place the carried block")
	}
	g.Tick()
	if len(w.placed) != 1 || w.placed[0] != (Pos{Y: 64}) || !m.Slot(SlotMainhand).Empty() {
		t.Fatalf("placed %v, hand %v", w.placed, m.Slot(SlotMainhand))
	}
}

func TestPlaceBlockWeightedPick(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{3}})
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:copper_golem")
	c := PlaceBlockConfig{RandomlyPlaceableBlocks: []WeightedBlock{
		{Block: PlacedBlock{Name: "a"}, Weight: 2},
		{Block: PlacedBlock{Name: "b"}, Weight: 2},
	}}
	g := NewPlaceBlock(w, m, c)
	g.Tick()
	if len(w.setTo) != 1 || w.setTo[0].Name != "b" {
		t.Fatalf("a draw of 3 over weights 2, 2 placed %v", w.setTo)
	}
}

func TestFindUnderwaterTreasureHeadsForTheWreck(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB11bMob(w, Vec3{X: 0.5, Y: 40, Z: 0.5}, "minecraft:dolphin")
	m.flags[FlagTreasure] = true
	m.wreck, m.hasWreck = Pos{X: 100, Y: 40}, true
	c := DefaultFindUnderwaterTreasure()
	c.StopDistance = 50
	g := NewFindUnderwaterTreasure(w, m, c)
	if !g.CanUse() {
		t.Fatal("a fed dolphin would not look for treasure")
	}
	g.Start()
	if !g.CanContinueToUse() {
		t.Fatal("found no wreck")
	}
	for range treasureCheck {
		g.Tick()
	}
	if n := m.nav; n.moves != 1 || n.to.X != 8 {
		t.Fatalf("aimed at %v after %d moves", n.to, n.moves)
	}
	if len(m.actorEv) != 1 || m.actorEv[0] != actorEventTreasure {
		t.Fatalf("actor events %v", m.actorEv)
	}
}

func TestSwimWithEntityCatchesUpThenMatches(t *testing.T) {
	w := newB11World(&seqRandom{floats: []float32{0.05}})
	m := newB11bMob(w, Vec3{X: 0.5, Y: 40, Z: 0.5}, "minecraft:dolphin")
	m.wet = map[ID]bool{m.id: true}
	p := newB11bMob(w, Vec3{X: 0.5, Y: 40, Z: 10.5}, "minecraft:player")
	p.id = 7
	p.flags[FlagSwimming] = true
	w.entities = append(w.entities, p)
	m.motion[p.id] = Vec3{Z: 1}
	g := NewSwimWithEntity(w, m, DefaultSwimWithEntity())
	if !g.CanUse() {
		t.Fatal("found nobody to swim with")
	}
	g.Start()
	g.Tick()
	// Swimming +z, the side is -x; the mob is straight behind, so the
	// share is 0.2 and the aim is 0.6 aside and two blocks ahead.
	if n := m.nav; n.moves != 1 || n.speed != 2.5 || n.to != (Vec3{X: -1, Y: 39, Z: 12}) {
		t.Fatalf("catch-up move %v at %v", n.to, n.speed)
	}
}

func TestRandomSearchAndDigRunsItsTriggers(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:sniffer")
	m.defaultOK = true
	c := DefaultRandomSearchAndDig()
	c.FindValidPositionRetries = 0
	c.ItemTable = "seeds"
	c.OnDiggingStart, c.OnSuccess, c.OnItemFound = []string{"dig"}, []string{"ok"}, []string{"item"}
	g := NewRandomSearchAndDig(w, m, c)
	if g.CanUse() {
		t.Fatal("dug with no tries left")
	}
	g.pos, g.state = Pos{Y: 64}, digSearching
	g.Start()
	m.nav.done = true
	g.Tick()
	if g.state != digDigging || !m.Flag(FlagDigging) {
		t.Fatalf("state %d after reaching the spot", g.state)
	}
	w.tick = g.digEnd
	g.Tick()
	g.Stop()
	if m.loot != 1 || g.state != digDone {
		t.Fatalf("loot %d, state %d", m.loot, g.state)
	}
	want := []string{"dig", "item", "ok"}
	got := m.fakeMob.events[len(m.fakeMob.events)-3:]
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events %v", m.fakeMob.events)
		}
	}
}

func TestEatMobPullsInAndEatsThePrey(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:frog")
	prey := newB11bMob(w, Vec3{X: 0.5, Y: 64, Z: 1}, "minecraft:slime")
	prey.id = 9
	w.entities = append(w.entities, prey)
	m.target, m.wanted = prey.id, true
	c := DefaultEatMob()
	c.LootTable = "slime_ball"
	g := NewEatMob(w, m, c)
	g.hasPath = true
	g.Start()
	g.Tick()
	if !m.Flag(FlagEatingMob) || m.pulled[prey.id].Z >= 0 {
		t.Fatalf("eating flag %v, pull %v", m.Flag(FlagEatingMob), m.pulled[prey.id])
	}
	g.Stop()
	if len(m.dropped) != 1 || len(m.removed) != 1 || m.removed[0] != prey.id {
		t.Fatalf("dropped %v, removed %v", m.dropped, m.removed)
	}
}
