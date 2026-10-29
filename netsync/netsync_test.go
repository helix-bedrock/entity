package netsync

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/helix-bedrock/protocol"
	"github.com/helix-bedrock/protocol/packet"
)

func TestRotationByte(t *testing.T) {
	cases := []struct {
		deg  float32
		want byte
	}{
		{0, 0}, {1.40625, 1}, {1.4, 0}, {90, 64}, {179, 127},
		{180, 128}, {-180, 128}, {-1, 0}, {-1.40625, 255}, {-90, 192},
		{360, 0}, {450, 64}, {-450, 192},
	}
	for _, c := range cases {
		if got := RotationByte(c.deg); got != c.want {
			t.Errorf("RotationByte(%v) = %d, want %d", c.deg, got, c.want)
		}
	}
}

func TestByteRoundTrip(t *testing.T) {
	for b := 0; b < 256; b++ {
		d := ByteDegrees(byte(b))
		if d < -180 || d >= 180 {
			t.Fatalf("ByteDegrees(%d) = %v, out of range", b, d)
		}
		if got := RotationByte(d); got != byte(b) {
			t.Fatalf("RotationByte(ByteDegrees(%d)) = %d", b, got)
		}
		// The protocol writer emits byte(deg / 1.40625).
		if got := byte(WireDegrees(byte(b)) / (360.0 / 256.0)); got != byte(b) {
			t.Fatalf("WireDegrees(%d) writes %d", b, got)
		}
	}
}

func TestSignificant(t *testing.T) {
	th := DefaultThresholds()
	last := Sent{Position: mgl32.Vec3{0, 64, 0}, Yaw: 10}
	moving := mgl32.Vec3{0.1, 0, 0}
	cases := []struct {
		name   string
		now    Sent
		motion mgl32.Vec3
		want   bool
	}{
		{"same", last, moving, false},
		{"step", Sent{Position: mgl32.Vec3{0.011, 64, 0}, Yaw: 10}, moving, true},
		{"tiny step while moving", Sent{Position: mgl32.Vec3{0.005, 64, 0}, Yaw: 10}, moving, false},
		{"tiny step after stopping", Sent{Position: mgl32.Vec3{0.005, 64, 0}, Yaw: 10}, mgl32.Vec3{}, true},
		{"turn 2 bytes", Sent{Position: last.Position, Yaw: 12}, moving, false},
		{"turn 3 bytes", Sent{Position: last.Position, Yaw: 13}, moving, true},
		// No wrap-around: byte 127 to 128 is 178.6 to -180 degrees.
		{"turn across 180", Sent{Position: last.Position, Yaw: 128}, moving, true},
	}
	for _, c := range cases {
		if got := Significant(last, c.now, c.motion, th); got != c.want {
			t.Errorf("%s: Significant = %v, want %v", c.name, got, c.want)
		}
	}
	wrap := Sent{Yaw: 127}
	if !Significant(wrap, Sent{Yaw: 128}, moving, th) {
		t.Error("a one-byte turn across 180 degrees must count as almost 360")
	}
}

func TestDelta(t *testing.T) {
	last := Sent{Position: mgl32.Vec3{1, 64, 2}, Pitch: 0, Yaw: 10, HeadYaw: 10}
	now := Sent{Position: mgl32.Vec3{1.5, 64, 2.25}, Pitch: 0, Yaw: 10, HeadYaw: 12, OnGround: true}
	if h := DeltaHeader(last, now, MoveFlags{}); h != 0x65 {
		t.Fatalf("header = %#x, want 0x65 (x, z, head yaw, on ground), the most common live header", h)
	}
	pk := Delta(7, last, now, MoveFlags{Teleported: true}, 0)
	if x, ok := pk.PositionX.Value(); !ok || x != 1.5 {
		t.Errorf("x = %v %v, want the full float 1.5", x, ok)
	}
	if _, ok := pk.PositionY.Value(); ok {
		t.Error("y did not change but was sent")
	}
	if _, ok := pk.RotationY.Value(); ok {
		t.Error("yaw did not change but was sent")
	}
	if h, ok := pk.RotationYHead.Value(); !ok || RotationByte(h) != 12 {
		t.Errorf("head yaw = %v %v, want byte 12", h, ok)
	}
	if !pk.OnGround || !pk.ForceMove || pk.ForceMoveLocalEntity || pk.ForceCompletion {
		t.Errorf("flags = %v %v %v %v", pk.OnGround, pk.ForceMove, pk.ForceMoveLocalEntity, pk.ForceCompletion)
	}
}

type throttleData struct {
	Rows [][7]float32 `json:"rows"`
}

// TestScoreLive checks Score against every movement update a live BDS sent.
// A row with score 0 may be an update BDS forced (on-ground changed), so
// only those may differ.
func TestScoreLive(t *testing.T) {
	raw, err := os.ReadFile("testdata/throttle.json")
	if err != nil {
		t.Fatal(err)
	}
	var d throttleData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	b := DefaultBandwidth()
	exact, forced := 0, 0
	for _, r := range d.Rows {
		got := Score(Distance(mgl32.Vec3{r[0], r[1], r[2]}, mgl32.Vec3{r[3], r[4], r[5]}), b)
		want := int64(r[6])
		switch {
		case got == want:
			exact++
		case want == 0:
			forced++
		default:
			t.Errorf("entity %v viewer %v: score %d, BDS %d", r[:3], r[3:6], got, want)
		}
	}
	if exact < 4700 {
		t.Errorf("only %d of %d rows exact", exact, len(d.Rows))
	}
	t.Logf("%d exact, %d forced to 0 by BDS", exact, forced)
}

func TestDue(t *testing.T) {
	if !Due(100, 0, 7) || !Due(7, 0, 7) || Due(6, 0, 7) || !Due(5, 5, 0) || Due(4, 5, 0) {
		t.Error("Due does not follow tick >= lastSent + score")
	}
	if Score(1e9, DefaultBandwidth()) != 7 || Score(0, DefaultBandwidth()) != 0 {
		t.Error("score must cap at 7 and be 0 next to the viewer")
	}
	if Score(10, Bandwidth{}) != 0 {
		t.Error("a zero distance setting must not throttle")
	}
}

func TestVanillaView(t *testing.T) {
	seen := map[ChunkPos]bool{}
	for _, c := range VanillaView(ChunkPos{}, nil) {
		if c.X*c.X+c.Z*c.Z > VanillaViewRadius2 || seen[c] {
			t.Fatalf("bad or repeated chunk %v", c)
		}
		seen[c] = true
	}
	if len(seen) != 57 {
		t.Fatalf("%d chunks, want 57", len(seen))
	}
	if !InVanillaView(mgl32.Vec3{0, 0, 0}, mgl32.Vec3{16 * 4, 0, 16}) || InVanillaView(mgl32.Vec3{}, mgl32.Vec3{16 * 3, 0, 16 * 3}) {
		t.Error("InVanillaView disagrees with dx² + dz² <= 17")
	}
	if c := ChunkAt(mgl32.Vec3{-0.5, 0, 15.9}); c != (ChunkPos{-1, 0}) {
		t.Errorf("ChunkAt = %v, want {-1 0}", c)
	}
}

type recorder struct{ batches [][]packet.Packet }

func (r *recorder) WriteBatch(pks []packet.Packet) {
	r.batches = append(r.batches, append([]packet.Packet(nil), pks...))
}

func item(id int32) protocol.ItemInstance {
	return protocol.ItemInstance{Stack: protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: id}, Count: 1}}
}

func names(pks []packet.Packet) []string {
	var out []string
	for _, pk := range pks {
		switch pk.(type) {
		case *packet.AddActor:
			out = append(out, "add")
		case *packet.RemoveActor:
			out = append(out, "remove")
		case *packet.MobArmourEquipment:
			out = append(out, "armour")
		case *packet.MobEquipment:
			out = append(out, "hand")
		case *packet.ActorEvent:
			out = append(out, "event")
		case *packet.MoveActorDelta:
			out = append(out, "delta")
		case *packet.MoveActorAbsolute:
			out = append(out, "absolute")
		case *packet.SetActorData:
			out = append(out, "data")
		case *packet.LevelSoundEvent:
			out = append(out, "sound")
		default:
			out = append(out, "?")
		}
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSyncOrderAndSpawn(t *testing.T) {
	s := New(VanillaOptions())
	r := &recorder{}
	s.AddViewer(1, r, mgl32.Vec3{0, 64, 0})
	bw := DefaultBandwidth()
	zombie := Entity{Spawn: Spawn{UniqueID: 10, RuntimeID: 10, Type: "minecraft:zombie"}, Mob: true, Bandwidth: &bw}
	zombie.Equipment.Armour[SlotHelmet] = item(5)
	zombie.Equipment.MainHand.Item = item(6)
	s.AddEntity(zombie, Pose{Position: mgl32.Vec3{2, 64, 2}, Yaw: 33})

	s.See(1, 10)
	s.Move(10, Pose{Position: mgl32.Vec3{2.5, 64, 2}, Yaw: 33, OnGround: true}, mgl32.Vec3{0.1, 0, 0}, MoveFlags{})
	sound := &packet.LevelSoundEvent{}
	s.Send(1, sound, PhaseSound)
	data := Data(10, nil, protocol.EntityProperties{}, 0)
	s.Broadcast(10, &data, PhaseData)
	s.Flush(1)

	want := []string{"add", "armour", "hand", "data", "delta", "sound"}
	if len(r.batches) != 1 || !eq(names(r.batches[0]), want) {
		t.Fatalf("batches = %v, want one batch %v", r.batches, want)
	}
	add := r.batches[0][0].(*packet.AddActor)
	if add.Yaw != ByteDegrees(RotationByte(33)) || add.Position != (mgl32.Vec3{2, 64, 2}) {
		t.Errorf("AddActor must use the last-sent pose, got yaw %v pos %v", add.Yaw, add.Position)
	}
	if last, _ := s.Last(10); last.Position != (mgl32.Vec3{2.5, 64, 2}) {
		t.Errorf("last-sent not moved after a delta was sent: %v", last.Position)
	}

	s.Unsee(1, 10)
	s.Flush(2)
	if got := names(r.batches[1]); !eq(got, []string{"remove"}) {
		t.Errorf("unsee sent %v", got)
	}
}

func TestSyncThrottleAndLastSent(t *testing.T) {
	s := New(VanillaOptions())
	near, far := &recorder{}, &recorder{}
	s.AddViewer(1, near, mgl32.Vec3{0, 64, 0})
	s.AddViewer(2, far, mgl32.Vec3{500, 64, 0})
	bw := DefaultBandwidth()
	s.AddEntity(Entity{Spawn: Spawn{UniqueID: 10, RuntimeID: 10}, Mob: true, Bandwidth: &bw}, Pose{Position: mgl32.Vec3{0, 64, 0}, OnGround: true})
	s.See(1, 10)
	s.See(2, 10)
	s.Flush(100)

	x := float32(0)
	for tick := uint64(101); tick <= 114; tick++ {
		x += 0.2
		s.Move(10, Pose{Position: mgl32.Vec3{x, 64, 0}, OnGround: true}, mgl32.Vec3{0.2, 0, 0}, MoveFlags{})
		s.Flush(tick)
	}
	count := func(r *recorder) int {
		n := 0
		for _, b := range r.batches {
			for _, name := range names(b) {
				if name == "delta" {
					n++
				}
			}
		}
		return n
	}
	// Score 7 far away: ticks 101 (never sent counts as 0) and 108.
	if n := count(far); n != 2 {
		t.Errorf("far viewer got %d deltas, want 2", n)
	}
	if n := count(near); n != 14 {
		t.Errorf("near viewer got %d deltas, want 14", n)
	}

	// With nobody seeing it, last-sent must stay where viewers last saw it.
	s.Unsee(1, 10)
	s.Unsee(2, 10)
	before, _ := s.Last(10)
	s.Move(10, Pose{Position: mgl32.Vec3{50, 64, 0}, OnGround: true}, mgl32.Vec3{}, MoveFlags{})
	s.Flush(200)
	if after, _ := s.Last(10); after != before {
		t.Errorf("last-sent moved with no viewer: %v -> %v", before.Position, after.Position)
	}
}

func TestSyncOptionsOff(t *testing.T) {
	s := New(Options{})
	r := &recorder{}
	s.AddViewer(1, r, mgl32.Vec3{})
	e := Entity{Spawn: Spawn{UniqueID: 3, RuntimeID: 3}, Mob: true}
	e.Equipment.MainHand.Item = item(9)
	s.AddEntity(e, Pose{})
	s.See(1, 3)
	s.Move(3, Pose{}, mgl32.Vec3{}, MoveFlags{})
	s.SetHand(3, false, Hand{Item: item(8)})
	s.Flush(1)
	if got := names(r.batches[0]); !eq(got, []string{"add", "absolute"}) {
		t.Errorf("with every option off got %v, want add then an absolute move", got)
	}
}

func TestRemoveEntityDropsMove(t *testing.T) {
	s := New(VanillaOptions())
	r := &recorder{}
	s.AddViewer(1, r, mgl32.Vec3{})
	s.AddEntity(Entity{Spawn: Spawn{UniqueID: 0, RuntimeID: 1}}, Pose{})
	s.See(1, 0)
	s.Move(0, Pose{Position: mgl32.Vec3{5, 0, 0}}, mgl32.Vec3{}, MoveFlags{})
	s.RemoveEntity(0)
	s.Flush(1)
	if got := names(r.batches[0]); !eq(got, []string{"add", "remove"}) {
		t.Errorf("got %v, want add then remove and no move", got)
	}
}

func BenchmarkSyncFlush(b *testing.B) {
	s := New(VanillaOptions())
	bw := DefaultBandwidth()
	for v := range 50 {
		s.AddViewer(ViewerID(v), &discard{}, mgl32.Vec3{float32(v), 64, 0})
	}
	for e := range 1000 {
		s.AddEntity(Entity{Spawn: Spawn{UniqueID: int64(e), RuntimeID: uint64(e)}, Mob: true, Bandwidth: &bw}, Pose{})
		for v := range 50 {
			s.See(ViewerID(v), EntityID(e))
		}
	}
	s.Flush(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		x := float32(i%100) * 0.3
		for e := range 1000 {
			s.Move(EntityID(e), Pose{Position: mgl32.Vec3{x, 64, float32(e)}, Yaw: x}, mgl32.Vec3{0.3, 0, 0}, MoveFlags{})
		}
		s.Flush(uint64(i + 1))
	}
}

type discard struct{}

func (discard) WriteBatch([]packet.Packet) {}
