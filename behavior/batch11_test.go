package behavior

import (
	"testing"

	"github.com/helix-bedrock/entity/path"
)

// b11World records placed blocks and answers the grief event and the base
// game version.
type b11World struct {
	*b8World
	placed  []Pos
	setTo   []PlacedBlock
	noGrief bool
}

func (w *b11World) SetBlock(p Pos, b PlacedBlock, _ ID) {
	w.placed, w.setTo = append(w.placed, p), append(w.setTo, b)
	w.blocks[p] = Block{Name: b.Name}
}
func (w *b11World) GriefAllowed(Pos, ID) bool        { return !w.noGrief }
func (w *b11World) BaseGameAtLeast(_, _, _ int) bool { return true }
func newB11World(r *seqRandom) *b11World             { return &b11World{b8World: newB8World(r)} }

// b11Mob adds the batch 11 hooks to b9Mob.
type b11Mob struct {
	*b9Mob
	playerHit ID
	counter   int32
	hasCount  bool
	walkTo    []Vec3
	probe     PathProbe
	probeOK   bool
	collided  bool
	node      func(p Pos) path.NodeType
}

func newB11Mob(w World, p Vec3, kind string) *b11Mob {
	return &b11Mob{b9Mob: newB9Mob(w, 1, p, kind), playerHit: NoID, probeOK: true,
		probe: PathProbe{Status: path.StatusComplete}}
}

func (m *b11Mob) LastHurtByPlayer() ID             { return m.playerHit }
func (m *b11Mob) RollCounter() (int32, bool)       { return m.counter, m.hasCount }
func (m *b11Mob) MayPlace(string, Pos) bool        { return true }
func (m *b11Mob) ProbePath(Vec3) (PathProbe, bool) { return m.probe, m.probeOK }
func (m *b11Mob) Collided() bool                   { return m.collided }
func (m *b11Mob) NodeType(_, p Pos) path.NodeType  { return m.node(p) }
func (m *b11Mob) WalkDirectly(_, to Vec3, _, _, _ int32) bool {
	m.walkTo = append(m.walkTo, to)
	return true
}

func TestRollSimulatesThePathOnFlatGround(t *testing.T) {
	w := newB11World(&seqRandom{floats: []float32{0.005}})
	m := newB11Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:panda")
	g := NewRoll(w, m, DefaultRoll())
	if !g.CanUse() || len(m.walkTo) != 1 {
		t.Fatalf("roll did not start: %v", m.walkTo)
	}
	// Facing yaw 0 the roll goes +z: 30 of its 33 steps push 0.2.
	if to := m.walkTo[0]; to.X != 0.5 || to.Z < 6.4 || to.Z > 6.6 {
		t.Fatalf("roll aimed at %+v", to)
	}
}

func TestRollOffALedgeSkipsTheChance(t *testing.T) {
	w := newB11World(&seqRandom{floats: []float32{0.5}})
	w.blocks[Pos{Y: 63, Z: 1}] = Block{Name: airName}
	m := newB11Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:panda")
	g := NewRoll(w, m, RollConfig{})
	if !g.CanUse() || len(m.walkTo) != 0 {
		t.Fatal("a panda at a drop did not roll")
	}
	w.blocks[Pos{Y: 64, Z: 1}] = Block{Name: "minecraft:oak_fence", Fence: true}
	if g.CanUse() {
		t.Fatal("rolled into a fence")
	}
}

func TestRollStepsHopAndStop(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB11Mob(w, Vec3{Y: 64}, "minecraft:panda")
	m.hasCount, m.ground = true, true
	g := NewRoll(w, m, DefaultRoll())
	g.Tick()
	if m.vel.Y != rollHop || m.vel.Z != rollSpeed {
		t.Fatalf("first step %+v", m.vel)
	}
	m.counter = rollHop1
	g.Tick()
	if m.vel.Z != 0 || m.vel.Y != rollHop {
		t.Fatalf("hop step %+v", m.vel)
	}
	m.counter = rollHop1 + 1
	g.Tick()
	if m.vel.Z != rollSpeed {
		t.Fatalf("push not kept: %+v", m.vel)
	}
	m.counter = rollSteps
	if g.CanContinueToUse() {
		t.Fatal("rolled past the last step")
	}
}

func TestSilverfishMergesIntoStone(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{0, 3}})
	w.griefing = true
	w.blocks[Pos{Y: 64, Z: 1}] = Block{Name: "minecraft:stone", Infested: "minecraft:infested_stone"}
	m := newB11Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:silverfish")
	m.nav.done = true
	g := NewSilverfishMerge(w, m)
	if !g.CanUse() || g.CanContinueToUse() {
		t.Fatal("silverfish did not pick the stone")
	}
	g.Start()
	if len(w.setTo) != 1 || w.setTo[0].Name != "minecraft:infested_stone" || m.removed != 1 || m.actorEv[0] != actorEventMerge {
		t.Fatalf("merge set %v removed %d events %v", w.setTo, m.removed, m.actorEv)
	}
}

func TestSilverfishWakesFriendsUntilTheCoinSaysStop(t *testing.T) {
	w := newB11World(&seqRandom{bools: []bool{false, true}})
	hit := newB11Mob(w, Vec3{}, "minecraft:player")
	hit.id = 9
	w.entities = []Actor{hit}
	m := newB11Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:silverfish")
	m.playerHit = 9
	host := Block{Name: "minecraft:infested_stone", Host: "minecraft:stone"}
	a, b, c := Pos{Y: 64, Z: 2}, Pos{X: 1, Y: 64}, Pos{Y: 65}
	for _, p := range []Pos{a, b, c} {
		w.blocks[p] = host
	}
	g := NewSilverfishWake(w, m)
	if !g.CanUse() {
		t.Fatal("hit silverfish did not wake its friends")
	}
	for range wakeDelay - 1 {
		g.Tick()
	}
	if len(w.placed) != 0 {
		t.Fatal("woke them early")
	}
	g.Tick()
	if len(w.placed) != 2 || w.placed[0] != a || w.placed[1] != b || w.setTo[0].Name != "minecraft:stone" {
		t.Fatalf("woke %v", w.placed)
	}
}

func TestLayEggLaysOnSand(t *testing.T) {
	w := newB11World(&seqRandom{ints: []int32{0, 2}})
	sand := Pos{X: 2, Y: 63}
	w.blocks[sand] = Block{Name: "minecraft:sand", Solid: true}
	m := newB11Mob(w, Vec3{X: 2, Y: 64}, "minecraft:turtle")
	m.flags[FlagHasEgg] = true
	c := DefaultLayEgg()
	c.SearchRange, c.SearchHeight, c.GoalRadius, c.OnLay = 4, 2, 1.5, []string{"laid_egg"}
	g := NewLayEgg(w, m, c)
	if !g.CanUse() || g.target != sand {
		t.Fatalf("no nest found: %v", g.target)
	}
	g.Start()
	for range 202 {
		g.Tick()
	}
	if len(w.setTo) != 1 || w.placed[0] != (Pos{X: 2, Y: 64}) || w.setTo[0] != (PlacedBlock{Name: turtleEggName, State: stateEggCount, Value: 2}) {
		t.Fatalf("laid %v at %v", w.setTo, w.placed)
	}
	if m.flags[FlagHasEgg] || m.flags[FlagLayingEgg] || len(m.fakeMob.events) != 1 {
		t.Fatalf("flags %v events %v", m.flags, m.fakeMob.events)
	}
	dust := 0
	for _, e := range w.events {
		if e == eventDestroyDust {
			dust++
		}
	}
	if dust != 40 {
		t.Fatalf("%d dust events, want 40", dust)
	}
}

func TestJumpToBlockPicksTurnsAndJumps(t *testing.T) {
	w := newB11World(&seqRandom{})
	m := newB11Mob(w, Vec3{X: 0.5, Y: 64, Z: 0.5}, "minecraft:frog")
	m.ground = true
	m.node = func(p Pos) path.NodeType {
		if p.Y == 63 {
			return path.NodeBlocked
		}
		return path.NodeOpen
	}
	g := NewJumpToBlock(w, m, DefaultJumpToBlock())
	if g.CanUse() {
		t.Fatal("jumped during the first cooldown")
	}
	w.tick = 200
	if !g.CanUse() {
		t.Fatal("frog did not want to jump")
	}
	g.Start()
	if g.state != jumpBlockPicking || len(g.cands) == 0 {
		t.Fatalf("state %d with %d candidates", g.state, len(g.cands))
	}
	w.tick++
	g.Tick()
	if g.state != jumpBlockTurning || g.target.Y != 64 {
		t.Fatalf("state %d target %v", g.state, g.target)
	}
	w.tick += jumpBlockTurnTime + 1
	g.Tick()
	g.Tick()
	if g.state != jumpBlockInAir || m.vel.Y <= 0 || !m.friction || m.played[len(m.played)-1] != jumpBlockSound {
		t.Fatalf("state %d velocity %+v", g.state, m.vel)
	}
	m.collided = true
	g.Tick()
	if g.state != jumpBlockDone || m.flags[FlagJumpGoal] {
		t.Fatal("landing did not end the jump")
	}
	w.blocks[Pos{Y: 64}] = Block{Name: honeyBlockName}
	w.tick = 1000
	if g.CanUse() {
		t.Fatal("jumped off honey")
	}
}
