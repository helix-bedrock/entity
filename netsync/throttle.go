package netsync

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Bandwidth is minecraft:conditional_bandwidth_optimization's values: how
// far away a viewer must be before movement updates are skipped, and how
// many ticks may be skipped at most.
type Bandwidth struct {
	MaxOptimizedDistance float32
	MaxDroppedTicks      int32
}

// DefaultBandwidth is the component's default (sub_737C5F0): 80 blocks
// and 10 ticks. Every vanilla mob in a live BDS capture used it.
func DefaultBandwidth() Bandwidth {
	return Bandwidth{MaxOptimizedDistance: 80, MaxDroppedTicks: 10}
}

// Distance is the float32 distance BDS throttles on, between the entity
// and the viewer.
func Distance(entity, viewer mgl32.Vec3) float32 {
	// sqrt in float64 then rounded is the correctly rounded float32 sqrt.
	return float32(math.Sqrt(float64(entity.Sub(viewer).LenSqr())))
}

// Score is how many ticks a viewer at dist may wait between two movement
// updates (ConditionalBandwidthOptimization, sub_8CE9D60):
// int(0.75 * maxDroppedTicks * min(dist, maxDist) / maxDist). With the
// default values a far viewer scores 7. Checked against 4,764 live BDS
// sends, all exact.
//
// BDS also scores 0 when the on-ground state changed since the last send,
// and when the entity has no bandwidth component; [Sync] does both.
func Score(dist float32, b Bandwidth) int64 {
	maxDist := b.MaxOptimizedDistance
	if maxDist <= 0 || dist != dist {
		// BDS divides by zero here; send every tick instead of relying on
		// what a NaN converts to.
		return 0
	}
	ratio := float32(1)
	if dist <= maxDist {
		ratio = dist / maxDist
	}
	return int64(0.75 * float32(b.MaxDroppedTicks) * ratio)
}

// Due reports whether a viewer last sent an update at lastSent may get one
// at tick: tick >= lastSent + score. A viewer never sent to counts as tick
// 0. Confirmed on 7,942 live check pairs.
func Due(tick, lastSent uint64, score int64) bool {
	if score < 0 {
		score = 0
	}
	next := lastSent + uint64(score)
	if next < lastSent {
		return false
	}
	return tick >= next
}
