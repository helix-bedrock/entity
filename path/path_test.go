package path

import "testing"

const (
	air Block = iota
	stone
	fence
	water
)

// testWorld is stone below y 64 and air above, with changes.
type testWorld struct {
	blocks map[Pos]Block
	infos  [4]BlockInfo
}

func newTestWorld() *testWorld {
	w := &testWorld{blocks: map[Pos]Block{}}
	w.infos[air] = BlockInfo{Kind: KindAir}
	w.infos[stone] = BlockInfo{Material: 3, Solid: true, SolidBlocking: true}
	w.infos[fence] = BlockInfo{Material: 2, Solid: true, SolidBlocking: true, Props: PropFence | 1<<26}
	w.infos[water] = BlockInfo{Kind: KindWater, Material: MaterialWater}
	return w
}

func (w *testWorld) Block(p Pos) Block {
	if b, ok := w.blocks[p]; ok {
		return b
	}
	if p.Y < 64 {
		return stone
	}
	return air
}

func (w *testWorld) Liquid(p Pos) Block {
	if w.Block(p) == water {
		return water
	}
	return air
}

func (w *testWorld) Info(b Block) *BlockInfo { return &w.infos[b] }

func (w *testWorld) CollisionBox(b Block, p Pos) Box {
	x, y, z := float32(p.X), float32(p.Y), float32(p.Z)
	switch b {
	case stone:
		return Box{Vec3{x, y, z}, Vec3{x + 1, y + 1, z + 1}}
	case fence:
		return Box{Vec3{x + 0.375, y, z + 0.375}, Vec3{x + 0.625, y + 1.5, z + 0.625}}
	}
	return Box{}
}

func (w *testWorld) MinY() int32 { return -64 }

func (w *testWorld) Pathable(b Block, _, _ Pos) bool { return b == air || b == water }

// zombie returns settings like a zombie's standing at block p.
func zombie(p Pos) *Settings {
	x, y, z := float32(p.X)+0.5, float32(p.Y), float32(p.Z)+0.5
	return &Settings{
		Box:        Box{Vec3{x - 0.3, y, z - 0.3}, Vec3{x + 0.3, y + 1.9, z + 0.3}},
		Width:      0.6,
		Height:     1.9,
		Pos:        Vec3{x, y, z},
		MinY:       -64,
		StepHeight: 1.2,
		Walk:       true,
	}
}

const zombieFlags = FlagCanPassDoors | FlagCanJump | FlagAvoidDamage

func TestStraightPath(t *testing.T) {
	w := newTestWorld()
	f := NewFinder()
	p := f.FindPathToBlock(w, zombie(Pos{0, 64, 0}), zombieFlags, Pos{6, 64, 0}, 35)
	if p == nil || p.Status != StatusComplete {
		t.Fatalf("no complete path: %+v", p)
	}
	if got := p.Points[len(p.Points)-1].Pos; got != (Pos{6, 64, 0}) {
		t.Fatalf("path ends at %v", got)
	}
	if len(p.Points) != 7 {
		t.Fatalf("path has %d points, want 7: %v", len(p.Points), p.Points)
	}
}

func TestPathAroundWall(t *testing.T) {
	w := newTestWorld()
	for z := int32(-3); z <= 3; z++ {
		for y := int32(64); y < 67; y++ {
			w.blocks[Pos{3, y, z}] = stone
		}
	}
	f := NewFinder()
	p := f.FindPathToBlock(w, zombie(Pos{0, 64, 0}), zombieFlags, Pos{6, 64, 0}, 35)
	if p == nil || p.Status != StatusComplete {
		t.Fatalf("no complete path: %+v", p)
	}
	for _, pt := range p.Points {
		if pt.Pos.X == 3 && pt.Pos.Z >= -3 && pt.Pos.Z <= 3 {
			t.Fatalf("path goes through the wall at %v", pt.Pos)
		}
	}
}

func TestPartialPathToEnclosedTarget(t *testing.T) {
	w := newTestWorld()
	for _, d := range []Pos{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}} {
		for y := int32(64); y < 67; y++ {
			w.blocks[Pos{10 + d.X, y, d.Z}] = stone
		}
	}
	f := NewFinder()
	p := f.FindPathToBlock(w, zombie(Pos{0, 64, 0}), zombieFlags, Pos{10, 64, 0}, 35)
	if p == nil || p.Status != StatusPartial {
		t.Fatalf("want a partial path, got %+v", p)
	}
}

func TestStepUpOneBlock(t *testing.T) {
	w := newTestWorld()
	for x := int32(3); x < 10; x++ {
		for z := int32(-5); z <= 5; z++ {
			w.blocks[Pos{x, 64, z}] = stone
		}
	}
	f := NewFinder()
	p := f.FindPathToBlock(w, zombie(Pos{0, 64, 0}), zombieFlags, Pos{6, 65, 0}, 35)
	if p == nil || p.Status != StatusComplete {
		t.Fatalf("no complete path up a step: %+v", p)
	}
	noJump := f.FindPathToBlock(w, zombie(Pos{0, 64, 0}), zombieFlags&^FlagCanJump, Pos{6, 65, 0}, 35)
	if noJump != nil && noJump.Status == StatusComplete {
		t.Fatal("a mob that cannot jump climbed a full block")
	}
}

func TestFenceBlocks(t *testing.T) {
	w := newTestWorld()
	for z := int32(-20); z <= 20; z++ {
		w.blocks[Pos{3, 64, z}] = fence
	}
	f := NewFinder()
	p := f.FindPathToBlock(w, zombie(Pos{0, 64, 0}), zombieFlags, Pos{6, 64, 0}, 35)
	if p != nil && p.Status == StatusComplete {
		t.Fatalf("path crossed a fence: %v", p.Points)
	}
}
