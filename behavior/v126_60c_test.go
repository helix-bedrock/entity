package behavior

import (
	"slices"
	"testing"
)

// solidWorld is a flatWorld whose stone has the block solid bit.
type solidWorld struct{ *flatWorld }

func (w solidWorld) Block(p Pos) Block {
	b := w.flatWorld.Block(p)
	b.BlockSolid = b.Solid
	return b
}

// gift is one DropGift call.
type gift struct {
	table string
	delay int32
	to    ID
}

// giftMob is a b2Mob with drop_item_for's hooks.
type giftMob struct {
	*b2Mob
	time     float32
	gifts    []gift
	teleport []Vec3
}

func (m *giftMob) TimeOfDay() float32 { return m.time }
func (m *giftMob) DropGift(t string, d int32, to ID) {
	m.gifts = append(m.gifts, gift{table: t, delay: d, to: to})
}
func (m *giftMob) Teleport(p Vec3) { m.teleport = append(m.teleport, p); m.pos = p }

func newDropTest() (*giftMob, *DropItemFor, *flatWorld) {
	fw := &flatWorld{r: &seqRandom{}}
	w := solidWorld{fw}
	cat := &giftMob{b2Mob: newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cat")}
	player := newB2Mob(w, 2, Vec3{X: 3.5, Y: 64, Z: 0.5}, "minecraft:player")
	fw.entities = []Actor{cat, player}
	c := DefaultDropItemFor()
	c.SearchRange, c.LootTable, c.OnDropAttempt = 5, "loot_tables/gift.json", "minecraft:drop_attempt"
	c.TargetRange = Vec3{X: 5, Y: 5, Z: 5}
	c.EntityTypes = []Filter{func(_ Mob, a Actor) bool { return a.Type() == "minecraft:player" }}
	return cat, NewDropItemFor(w, cat, c), fw
}

func TestDropItemForGivesGift(t *testing.T) {
	cat, g, _ := newDropTest()
	if !g.CanUse() {
		t.Fatal("did not start with the cooldown at 0")
	}
	g.Start()
	if g.target != 2 || g.aim != (Vec3{X: 3, Y: 63, Z: 0}) {
		t.Fatalf("target %v spot %v, want the player and the block under it", g.target, g.aim)
	}
	if cat.nav.to != (Vec3{X: 3, Y: 64, Z: 0}) {
		t.Fatalf("walked to %v, want the spot plus the offset", cat.nav.to)
	}
	// The spot is more than the minimum teleport distance (squared) away.
	g.Tick()
	if len(cat.teleport) != 1 || cat.teleport[0] != (Vec3{X: 3, Y: 64, Z: 0}) {
		t.Fatalf("teleports %v, want one to the spot plus the offset", cat.teleport)
	}
	g.Tick()
	if len(cat.gifts) != 1 || cat.gifts[0] != (gift{table: "loot_tables/gift.json", to: 2}) {
		t.Fatalf("gifts %v, want one roll of the table for the player", cat.gifts)
	}
	if g.CanContinueToUse() {
		t.Fatal("kept going after the drop")
	}
	g.Stop()
	if !slices.Contains(cat.events, "minecraft:drop_attempt") {
		t.Fatalf("events %v, want on_drop_attempt", cat.events)
	}
}

func TestDropItemForTimeWindowAndCooldown(t *testing.T) {
	cat, g, _ := newDropTest()
	cfg := g.Config()
	cfg.TimeOfDayRange = Range[float32]{0.7, 0.8}
	g.SetConfig(cfg)
	cat.time = 0.5
	g.Start()
	if g.CanContinueToUse() {
		t.Fatal("found a target outside the time window")
	}
	// cooldown 0.25 s = 5 ticks
	for i := 0; i < 5; i++ {
		if g.CanUse() {
			t.Fatalf("started during the cooldown, call %d", i)
		}
	}
	if !g.CanUse() {
		t.Fatal("did not start after the cooldown")
	}
}

// moved is one TakeItems or PlaceItems call.
type moved struct {
	place, simulate bool
	at              Pos
}

// golemMob is a b2Mob with transport_items' hooks.
type golemMob struct {
	*b2Mob
	containers map[Pos]TransportContainer
	holds      bool
	opened     []Pos
	closed     int
	moves      []moved
}

func (m *golemMob) MatchesBlock(Pos, []string) bool { return true }
func (m *golemMob) Containers(dst []TransportContainer, b Box) []TransportContainer {
	for _, c := range m.containers {
		p := vecOf(c.Pos)
		if p.X >= b.Min.X && p.X <= b.Max.X && p.Y >= b.Min.Y && p.Y <= b.Max.Y && p.Z >= b.Min.Z && p.Z <= b.Max.Z {
			dst = append(dst, c)
		}
	}
	return dst
}
func (m *golemMob) Container(p Pos) (TransportContainer, bool) {
	c, ok := m.containers[p]
	return c, ok
}
func (m *golemMob) HoldsItem() bool     { return m.holds }
func (m *golemMob) SeesBlock(Pos) bool  { return true }
func (m *golemMob) OpenContainer(p Pos) { m.opened = append(m.opened, p) }
func (m *golemMob) CloseContainer()     { m.closed++ }
func (m *golemMob) TakeItems(p Pos, _ TransportRules, sim bool) bool {
	m.moves = append(m.moves, moved{at: p, simulate: sim})
	if !sim {
		m.holds = true
	}
	return true
}
func (m *golemMob) PlaceItems(p Pos, _ TransportRules, sim bool) bool {
	m.moves = append(m.moves, moved{place: true, at: p, simulate: sim})
	if !sim {
		m.holds = false
	}
	return true
}

func newGolem(w World) *golemMob {
	chest := Pos{X: 5, Y: 64}
	return &golemMob{b2Mob: newB2Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:copper_golem"),
		containers: map[Pos]TransportContainer{chest: {Pos: chest, Kind: 2, CanOpen: true}}}
}

func TestTransportItemsTakesFromChest(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newGolem(w)
	g := NewTransportItems(w, m, DefaultTransportItems())
	if !g.CanUse() {
		t.Fatal("did not start")
	}
	g.Start()
	g.Tick()
	if g.state != transportWalk || m.nav.to != (Vec3{X: 5.5, Y: 64.5, Z: 0.5}) {
		t.Fatalf("state %d walking to %v, want the chest's middle", g.state, m.nav.to)
	}
	m.pos = Vec3{X: 5.5, Y: 64, Z: 1.2}
	g.Tick()
	if g.state != transportArrive {
		t.Fatalf("state %d, want arrived next to the chest", g.state)
	}
	g.Tick()
	if g.state != transportInteract || len(m.opened) != 1 || !slices.Contains(m.events, "minecraft:transport_items.start_take_succeed") {
		t.Fatalf("state %d opened %v events %v, want the chest open and a take announced", g.state, m.opened, m.events)
	}
	g.Tick()
	if len(m.moves) != 1 {
		t.Fatalf("moves %v, want only the simulated take before the interaction time", m.moves)
	}
	w.tick += 60
	g.Tick()
	if !m.holds || len(m.moves) != 2 || m.moves[1].simulate {
		t.Fatalf("holds %v moves %v, want the real take", m.holds, m.moves)
	}
	if g.state != transportSearch || len(g.visited) != 0 {
		t.Fatalf("state %d visited %v, want a new search after the hand changed", g.state, g.visited)
	}
}

func TestTransportItemsRestsWithNothingInRange(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newGolem(w)
	m.containers = nil
	g := NewTransportItems(w, m, DefaultTransportItems())
	g.Start()
	g.Tick()
	if g.CanContinueToUse() {
		t.Fatal("kept going with no container in range")
	}
	w.tick = 400
	if g.CanUse() {
		t.Fatal("started again before the idle cooldown ended")
	}
	w.tick = 401
	if !g.CanUse() {
		t.Fatal("did not start after the idle cooldown")
	}
}

func TestTransportItemsResetsOnHandChange(t *testing.T) {
	w := &flatWorld{r: &seqRandom{}}
	m := newGolem(w)
	g := NewTransportItems(w, m, DefaultTransportItems())
	g.Start()
	g.Tick()
	m.holds = true
	g.Tick()
	if g.state != transportSearch || g.hasTarget || len(g.visited) != 0 {
		t.Fatalf("state %d target %v visited %v, want a reset", g.state, g.hasTarget, g.visited)
	}
}
