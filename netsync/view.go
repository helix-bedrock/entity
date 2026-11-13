package netsync

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// ChunkPos is a chunk column position.
type ChunkPos struct{ X, Z int32 }

// ChunkAt is the chunk a position is in: floor(x)>>4, floor(z)>>4, as BDS
// takes a player's view centre (sub_CF12A70).
func ChunkAt(p mgl32.Vec3) ChunkPos {
	return ChunkPos{X: floorBlock(p[0]) >> 4, Z: floorBlock(p[2]) >> 4}
}

func floorBlock(f float32) int32 {
	v := math.Floor(float64(f))
	if v != v {
		return 0
	}
	return int32(max(min(v, math.MaxInt32), math.MinInt32))
}

// VanillaViewRadius2 is the squared chunk radius of BDS's view area: every
// chunk with dx² + dz² <= 17 around the centre, 57 chunks. Read live from
// the Level's offset table (Level slot 264, sub_CE3A100 -> sub_CFE3060);
// it did not change with tick-distance.
const VanillaViewRadius2 = 17

// VanillaViewOffsets are the 57 chunk offsets of BDS's view area, in the
// order the live Level held them.
func VanillaViewOffsets() [57]ChunkPos {
	return [57]ChunkPos{
		{1, 4}, {-2, 3}, {1, 3}, {-2, -2}, {2, 2}, {4, 0}, {1, -1}, {0, 2}, {-1, 1}, {2, -2},
		{-2, 0}, {3, -2}, {-2, -1}, {1, -2}, {-1, 4}, {-4, 0}, {4, -1}, {-2, -3}, {-2, 1}, {0, -3},
		{2, -3}, {3, 1}, {0, 0}, {1, -4}, {1, 1}, {-1, 0}, {2, -1}, {0, -2}, {-2, 2}, {-1, -3},
		{0, 1}, {2, 0}, {3, 0}, {-3, -1}, {0, -1}, {1, 2}, {-4, 1}, {1, -3}, {2, 1}, {-1, -4},
		{-3, 1}, {1, 0}, {-1, 2}, {-1, -2}, {0, 3}, {-1, -1}, {-1, 3}, {4, 1}, {-4, -1}, {-3, -2},
		{2, 3}, {-3, 0}, {3, -1}, {0, -4}, {0, 4}, {-3, 2}, {3, 2},
	}
}

// VanillaView appends to dst the chunks BDS lets a player at centre see
// entities in. It is only a helper: the engine decides who sees what.
//
// BDS marks every entity in these chunks as seen each tick; on the
// player's tick a newly seen entity gets AddActor and one no longer seen
// gets RemoveActor (sub_CF2AE10, sub_A14B5B0). BDS also adds a second
// centre from a component that was not identified.
func VanillaView(centre ChunkPos, dst []ChunkPos) []ChunkPos {
	for _, o := range VanillaViewOffsets() {
		dst = append(dst, ChunkPos{X: centre.X + o.X, Z: centre.Z + o.Z})
	}
	return dst
}

// InVanillaView reports whether an entity at entity is in the view area of
// a player at viewer.
func InVanillaView(viewer, entity mgl32.Vec3) bool {
	v, e := ChunkAt(viewer), ChunkAt(entity)
	dx, dz := int64(e.X)-int64(v.X), int64(e.Z)-int64(v.Z)
	return dx*dx+dz*dz <= VanillaViewRadius2
}
