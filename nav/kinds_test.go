package nav

import (
	"testing"

	"github.com/helix-bedrock/entity/path"
)

type actor map[int]bool

func (a actor) Flag(n int) bool        { return a[n] }
func (a actor) SetFlag(n int, on bool) { a[n] = on }

func TestInitFlags(t *testing.T) {
	cases := []struct {
		k               Kind
		swim, walk, fly bool
	}{
		{KindWalk, false, true, false},
		{KindSwim, true, false, false},
		{KindFly, false, false, true},
		{KindHover, false, false, true},
	}
	for _, c := range cases {
		a := actor{}
		m := &Mob{Actor: a}
		NewKind(c.k, &Description{}).Init(m)
		if a[ActorCanSwim] != c.swim || a[ActorCanWalk] != c.walk || a[ActorCanFly] != c.fly {
			t.Errorf("kind %d: flags %v", c.k, a)
		}
	}
}

func TestFlyGravity(t *testing.T) {
	w := newFlatWorld()
	a := actor{ActorHasGravity: true}
	c := &control{}
	m := walker(0.5, 0.5, c)
	m.Actor = a
	m.Pos.Y, m.Box.Min.Y = 70, 70
	n := NewKind(KindFly, &Description{})
	n.Init(m)
	p := &path.Path{Points: []path.Point{{Pos: path.Pos{X: 3, Y: 70}, Type: path.NodeAir}}}
	n.Follow(w, m, p, 1)
	n.Tick(w, m)
	if a[ActorHasGravity] || !c.set {
		t.Fatalf("following: gravity %v, wanted set %v", a[ActorHasGravity], c.set)
	}
	n.Stop(m)
	if !a[ActorHasGravity] {
		t.Fatal("stop did not give gravity back")
	}
}

func TestFloatNeverPaths(t *testing.T) {
	n := NewKind(KindFloat, &Description{})
	if n.MoveTo(newFlatWorld(), walker(0.5, 0.5, nil), nil, path.Vec3{X: 5, Y: 64}, 1) {
		t.Fatal("float navigation moved")
	}
}

func TestSwimFlags(t *testing.T) {
	n := NewKind(KindSwim, &Description{AvoidWater: true, AvoidDamageBlocks: true})
	if n.Flags != FlagCanFloat {
		t.Fatalf("flags = %#x", n.Flags)
	}
}

func TestRaySkipsStart(t *testing.T) {
	var seen []path.Pos
	ok := ray(path.Vec3{X: 0.5, Y: 0.5, Z: 0.5}, path.Vec3{X: 2.5, Y: 0.5, Z: 0.5}, func(p path.Pos) bool {
		seen = append(seen, p)
		return true
	})
	if !ok || len(seen) != 2 || seen[0].X != 1 || seen[1].X != 2 {
		t.Fatalf("ok %v, seen %v", ok, seen)
	}
}
