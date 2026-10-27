package behavior

import "testing"

// b3World adds the column and pathing answers to blockWorld.
type b3World struct{ *blockWorld }

func (w *b3World) AboveTopSolid(int32, int32) int32 { return 64 }
func (w *b3World) Passable(p Pos, _ ID) bool        { return !w.Block(p).BlocksMotion }

func newB3World(r *seqRandom) *b3World { return &b3World{blockWorld: newBlockWorld(r)} }

// b3Mob adds a move control, families and a trading player to swimMob.
type b3Mob struct {
	*swimMob
	families []string
	trader   ID
}

func (m *b3Mob) MoveWanted() (bool, bool) { return false, true }
func (m *b3Mob) TradingPlayer() ID        { return m.trader }
func (m *b3Mob) MaxHeadXRot() float32     { return 40 }
func (m *b3Mob) HasFamily(f string) bool {
	for _, x := range m.families {
		if x == f {
			return true
		}
	}
	return false
}

func newB3Mob(w World, id ID, p Vec3, kind string) *b3Mob {
	m := &b3Mob{swimMob: newSwimMob(w, p), trader: NoID}
	m.id, m.kind = id, kind
	return m
}

func TestRandomSitTimesAndFlags(t *testing.T) {
	w := newB3World(&seqRandom{ints: []int32{10, 7, 9}, floats: []float32{0.05}})
	w.tick = 100
	m := newB3Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cat")
	g := NewRandomSit(w, m, DefaultRandomSit())
	if g.CanUse() || g.next != 100+2000+10 {
		t.Fatalf("first start at %d, want 2110", g.next)
	}
	w.tick = 2111
	if !g.CanUse() {
		t.Fatal("random_sitting did not start")
	}
	g.Start()
	if !m.Flag(FlagTransitionSitting) || g.sitUntil != 2111+200 {
		t.Fatalf("start: flag %v until %d", m.Flag(FlagTransitionSitting), g.sitUntil)
	}
	if !g.CanContinueToUse() {
		t.Fatal("stopped before min_sit_time")
	}
	g.Stop()
	if !m.Flag(FlagSitting) || g.next != 2111+2000+7+9 {
		t.Fatalf("stop: sitting %v next %d", m.Flag(FlagSitting), g.next)
	}
}

func TestLookAtTradingNeverStarts(t *testing.T) {
	w := newB3World(&seqRandom{})
	m := newB3Mob(w, 1, Vec3{}, "minecraft:villager_v2")
	p := newMob(w, 2, Vec3{X: 2}, "minecraft:player")
	w.entities = []Actor{m, p}
	m.trader = 2
	g := NewLookAtTrading(w, m, DefaultLookAtTarget())
	if g.CanUse() {
		t.Fatal("look_at_trading_player started")
	}
	if g.target != 2 {
		t.Fatalf("look target %d, want the trading player", g.target)
	}
}

func TestLookAtTargetOnlyLooksAtTarget(t *testing.T) {
	w := newB3World(&seqRandom{})
	m := newB3Mob(w, 1, Vec3{}, "minecraft:piglin")
	a, b := newB3Mob(w, 2, Vec3{X: 1}, "minecraft:pig"), newB3Mob(w, 3, Vec3{X: 2}, "minecraft:player")
	w.entities = []Actor{m, a, b}
	m.target = 3
	g := NewLookAtTarget(w, m, DefaultLookAtTarget())
	if !g.CanUse() || g.target != 3 {
		t.Fatalf("look target %d, want the mob's target 3", g.target)
	}
}

func TestFacingAndTurns(t *testing.T) {
	for _, c := range []struct {
		d    Vec3
		want uint8
	}{{Vec3{Y: 2, X: 1}, dirUp}, {Vec3{X: -3, Z: 1}, dirWest}, {Vec3{X: 1, Z: 1}, dirSouth}, {Vec3{}, dirUp}} {
		if got := facing(c.d, c.d.X*c.d.X+c.d.Y*c.d.Y+c.d.Z*c.d.Z); got != c.want {
			t.Errorf("facing %v = %d, want %d", c.d, got, c.want)
		}
	}
	w := newB3World(&seqRandom{ints: []int32{1, 0}})
	g := NewMoveToRandomBlock(w, newB3Mob(w, 1, Vec3{}, "x"), DefaultMoveToRandomBlock())
	if g.turn(dirDown) != dirUp || g.turn(dirNorth) != dirEast || g.turn(dirNorth) != dirWest {
		t.Fatal("stuck turns differ from BDS's tables")
	}
}

func TestMoveToRandomBlockAimsHalfFollowRange(t *testing.T) {
	w := newB3World(&seqRandom{ints: []int32{16, 16, 8}})
	m := newB3Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:pig")
	g := NewMoveToRandomBlock(w, m, DefaultMoveToRandomBlock())
	if !g.CanUse() || g.target.Y != 64 {
		t.Fatalf("target %v", g.target)
	}
	m.nav.done = true
	g.Start()
	g.target = Vec3{X: 40.5, Y: 64, Z: 0.5}
	g.last = Vec3{X: 10}
	g.Tick()
	// follow_range 16: the aim is 8 blocks along the way.
	if m.nav.to != (Vec3{X: 8, Y: 64}) {
		t.Fatalf("aim %v, want (8, 64, 0)", m.nav.to)
	}
}

func TestFollowMobDefaultFilterAndPreferred(t *testing.T) {
	w := newB3World(&seqRandom{})
	m := newB3Mob(w, 1, Vec3{}, "minecraft:cod")
	same := newB3Mob(w, 2, Vec3{X: 1}, "minecraft:cod")
	same.families = []string{"cod"}
	squid := newB3Mob(w, 3, Vec3{X: 2}, "minecraft:squid")
	squid.families = []string{"squid"}
	turtle, frog := newB3Mob(w, 4, Vec3{X: 3}, "minecraft:turtle"), newB3Mob(w, 5, Vec3{X: 4}, "minecraft:frog")
	w.entities = []Actor{m, same, squid, turtle, frog}
	c := DefaultFollowMob()
	c.SearchRange, c.PreferredType = 8, "minecraft:frog"
	g := NewFollowMob(w, m, c)
	if !g.CanUse() || g.target != 5 {
		t.Fatalf("follow target %d, want the preferred frog", g.target)
	}
	c.PreferredType = ""
	w.r.ints = []int32{0, 0}
	g = NewFollowMob(w, m, c)
	if !g.CanUse() || g.target != 4 {
		t.Fatalf("follow target %d, want the turtle (cod and squid filtered out)", g.target)
	}
}

func TestAvoidBlockRunsAwayAndSprints(t *testing.T) {
	w := newB3World(&seqRandom{ints: []int32{0, 20, 20, 8}})
	cactus := Pos{X: -1, Y: 64}
	w.blocks[cactus] = Block{Name: "minecraft:cactus", HasCollision: true}
	m := newB3Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:rabbit")
	c := DefaultAvoidBlock()
	c.SearchRange, c.SearchHeight, c.SprintSpeed = 4, 1, 1.5
	c.TargetBlocks, c.OnEscape = []string{"minecraft:cactus"}, []string{"escaped"}
	g := NewAvoidBlock(w, m, c)
	if !g.CanUse() || g.block != cactus {
		t.Fatalf("avoid_block block %v", g.block)
	}
	if blockDist2(cactus, blockOf(g.flee)) < blockDist2(cactus, blockOf(m.pos)) {
		t.Fatalf("flee %v is closer to the cactus", g.flee)
	}
	g.Start()
	if !m.Flag(FlagAvoidingBlock) {
		t.Fatal("avoiding flag not set")
	}
	g.Tick()
	if m.nav.speed != 1.5 {
		t.Fatalf("speed %v, want sprint near the block", m.nav.speed)
	}
	g.Stop()
	if m.Flag(FlagAvoidingBlock) || len(m.events) != 1 {
		t.Fatalf("stop: flag %v events %v", m.Flag(FlagAvoidingBlock), m.events)
	}
}

func TestMoveToBlockReachesAndFires(t *testing.T) {
	w := newB3World(&seqRandom{ints: []int32{5}})
	bed := Pos{X: 3, Y: 64}
	w.blocks[bed] = Block{Name: "minecraft:bed", HasCollision: true}
	m := newB3Mob(w, 1, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:cat")
	c := DefaultMoveToBlock()
	c.SearchRange, c.TargetBlocks, c.OnReach = 4, []string{"minecraft:bed"}, []string{"reached"}
	c.TargetOffset = Vec3{Y: -0.5}
	g := NewMoveToBlock(w, m, c)
	if !g.CanUse() || g.target != bed || g.interval != 25 {
		t.Fatalf("target %v interval %d", g.target, g.interval)
	}
	g.Start()
	if m.nav.to != (Vec3{X: 3.5, Y: 64, Z: 0.5}) {
		t.Fatalf("walking to %v, want the block centre plus the offset", m.nav.to)
	}
	m.pos = Vec3{X: 3.5, Y: 63.5, Z: 0.5}
	g.Tick()
	g.Tick()
	if !g.reached || len(m.events) != 1 || m.events[0] != "reached" {
		t.Fatalf("reached %v events %v", g.reached, m.events)
	}
}

func TestParseSelection(t *testing.T) {
	if m, ok := ParseSelection("RANDOM"); !ok || m != SelectRandom {
		t.Fatal("random not parsed")
	}
	if _, ok := ParseSelection("farthest"); ok {
		t.Fatal("unknown method accepted")
	}
}
