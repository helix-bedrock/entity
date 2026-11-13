package state

import "github.com/helix-bedrock/entity/path"

// PathCost is one of preferred_path's preferred_path_blocks: the cost of
// walking on blocks matching the descriptors.
type PathCost struct {
	Cost   float32
	Blocks []string
}

// PreferredPathConfig is minecraft:preferred_path (1.21.3
// PreferredPathDescription constructor 0x1405CBF10: max_fall_blocks 3,
// the costs 0).
type PreferredPathConfig struct {
	DefaultBlockCost, JumpCost float32
	MaxFallBlocks              int32
	Costs                      []PathCost
}

// DefaultPreferredPath is the BDS default.
func DefaultPreferredPath() PreferredPathConfig { return PreferredPathConfig{MaxFallBlocks: 3} }

// Path builds the pathfinder's component; match turns a descriptor list
// into the engine's block test.
func (c *PreferredPathConfig) Path(match func(blocks []string) func(path.Block) bool) *path.PreferredPath {
	p := &path.PreferredPath{DefaultBlockCost: c.DefaultBlockCost, JumpCost: c.JumpCost, MaxFallBlocks: c.MaxFallBlocks}
	for _, e := range c.Costs {
		p.Costs = append(p.Costs, path.CostEntry{Cost: e.Cost, Blocks: match(e.Blocks)})
	}
	return p
}

// BreakDoorConfig is minecraft:annotation.break_door: the navigation's
// door breaking (nav.BreakDoors) and the ticks a break takes. Its
// defaults were not read.
type BreakDoorConfig struct {
	BreakTime     int32
	MinDifficulty int32
}
