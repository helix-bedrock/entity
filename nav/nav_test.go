package nav

import (
	"testing"

	"github.com/helix-bedrock/entity/path"
)

const (
	air path.Block = iota
	stone
)

// flatWorld is stone at y < 64 and air above.
type flatWorld struct{ infos [2]path.BlockInfo }

func newFlatWorld() *flatWorld {
	w := &flatWorld{}
	w.infos[air] = path.BlockInfo{Kind: path.KindAir}
	w.infos[stone] = path.BlockInfo{Solid: true, SolidBlocking: true}
	return w
}

func (w *flatWorld) Block(p path.Pos) path.Block {
	if p.Y < 64 {
		return stone
	}
	return air
}
func (w *flatWorld) Liquid(path.Pos) path.Block                { return air }
func (w *flatWorld) Info(b path.Block) *path.BlockInfo         { return &w.infos[b] }
func (w *flatWorld) MinY() int32                               { return -64 }
func (w *flatWorld) Pathable(b path.Block, _, _ path.Pos) bool { return b == air }
func (w *flatWorld) CollisionBox(b path.Block, p path.Pos) path.Box {
	if b == air {
		return path.Box{}
	}
	f := path.Vec3{X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z)}
	return path.Box{Min: f, Max: path.Vec3{X: f.X + 1, Y: f.Y + 1, Z: f.Z + 1}}
}
func (w *flatWorld) Door(path.Block) (int32, bool, bool) { return 0, false, false }
func (w *flatWorld) HeightRange() (int32, int32)         { return -64, 320 }
func (w *flatWorld) Height(_, _ int32) (int32, bool)     { return 64 + 64, true }
func (w *flatWorld) IsDay() bool                         { return true }

type control struct {
	wanted path.Vec3
	set    bool
}

func (c *control) SetWantedPosition(p path.Vec3, _ float32) { c.wanted, c.set = p, true }
func (c *control) ClearWanted()                             { c.set = false }

func walker(x, z float32, mc MoveControl) *Mob {
	return &Mob{
		Pos:         path.Vec3{X: x, Y: 64, Z: z},
		Box:         path.Box{Min: path.Vec3{X: x - 0.3, Y: 64, Z: z - 0.3}, Max: path.Vec3{X: x + 0.3, Y: 65.9, Z: z + 0.3}},
		Width:       0.6,
		Height:      1.9,
		CanWalk:     true,
		OnGround:    true,
		MoveControl: mc,
	}
}

func straight(n int32) *path.Path {
	p := &path.Path{Status: path.StatusComplete}
	for i := int32(0); i < n; i++ {
		p.Points = append(p.Points, path.Point{Pos: path.Pos{X: i, Y: 64}, Type: path.NodeOpen})
	}
	return p
}

func TestFollowAimsAtNextNode(t *testing.T) {
	w := newFlatWorld()
	c := &control{}
	m := walker(0.5, 0.5, c)
	n := New(&Description{})
	if !n.Follow(w, m, straight(5), 1) {
		t.Fatal("Follow refused the path")
	}
	n.Tick(w, m)
	if _, i := n.Path(); i != 1 {
		t.Fatalf("index = %d, want 1 (node 0 reached)", i)
	}
	if !c.set || c.wanted != (path.Vec3{X: 1.5, Y: 64, Z: 0.5}) {
		t.Fatalf("wanted = %+v, want {1.5 64 0.5}", c.wanted)
	}
}

func TestSamePathKeepsProgress(t *testing.T) {
	w := newFlatWorld()
	m := walker(0.5, 0.5, &control{})
	n := New(&Description{})
	n.Follow(w, m, straight(5), 1)
	n.Tick(w, m)
	n.Follow(w, m, straight(5), 1)
	if _, i := n.Path(); i != 1 {
		t.Fatalf("index = %d, want 1 kept", i)
	}
}

func TestStuckStops(t *testing.T) {
	w := newFlatWorld()
	m := walker(0.5, 0.5, &control{})
	n := New(&Description{})
	n.Follow(w, m, straight(20), 1)
	for range DefaultStuckInterval + 1 {
		n.Tick(w, m)
	}
	if !n.Done() {
		t.Fatal("a mob that never moved kept its path")
	}
}

func TestAvoidSunTrims(t *testing.T) {
	w := newFlatWorld()
	m := walker(0.5, 0.5, &control{})
	m.Box.Min.Y = 0 // in the shade, deep below the heightmap
	n := New(&Description{AvoidSun: true})
	if n.Follow(w, m, straight(5), 1) {
		t.Fatal("a path that is all in the sun was kept")
	}
}

func TestPathFlags(t *testing.T) {
	d := Description{CanBreach: true, CanPathOverWater: true}
	got := d.Flags().PathFlags()
	if got != path.FlagCanBreach|path.FlagPathOverWater {
		t.Fatalf("PathFlags = %#x", got)
	}
}

func TestWalkDirectly(t *testing.T) {
	w := newFlatWorld()
	m := walker(0.5, 0.5, nil)
	if !walkDirectly(w, m, path.Vec3{X: 0.5, Y: 64, Z: 0.5}, path.Vec3{X: 6.5, Y: 64, Z: 3.5}, 3, 3, 3) {
		t.Fatal("open flat ground is not walkable")
	}
}
