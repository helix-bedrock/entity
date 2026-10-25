package spawn

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Pool is a population_control pool; each has its own mob cap.
type Pool int8

// The pools in BDS's order. PoolNone is any other population_control: such
// rules never spawn naturally.
const (
	PoolNone Pool = iota - 1
	PoolAnimal
	PoolMonster
	PoolWaterAnimal
	PoolVillager
	PoolAmbient
	PoolCat
	PoolPillager
	numPools
)

func poolOf(name string) Pool {
	switch name {
	case "animal":
		return PoolAnimal
	case "monster":
		return PoolMonster
	case "water_animal":
		return PoolWaterAnimal
	case "villager":
		return PoolVillager
	case "ambient":
		return PoolAmbient
	case "cat":
		return PoolCat
	case "pillager":
		return PoolPillager
	}
	return PoolNone
}

// Herd is one minecraft:herd entry.
type Herd struct {
	Min, Max          int32
	InitialEvent      string
	InitialEventCount int32
	Event             string
	EventSkipCount    int32
}

// Permute is one minecraft:permute_type entry.
type Permute struct {
	Weight, Guaranteed int32
	Type               string
}

// Rule is one condition of a spawn rules file: BDS's MobSpawnerData. The
// zero value is not useful; Load fills the BDS defaults.
type Rule struct {
	Identifier string
	Pool       Pool
	Weight     int32
	// Rarity rejects the cluster unless Int(Rarity) is 0.
	Rarity int32

	Surface, Underground, Underwater, Lava bool
	// AllowBubble is false for minecraft:disallow_spawns_in_bubble.
	AllowBubble bool

	BrightnessMin, BrightnessMax int32
	AdjustForWeather             bool
	MoonMin, MoonMax             float32
	HeightMin, HeightMax         int32
	// Density is the density_limit: [0] surface, [1] underground; −1 is no
	// limit.
	Density                      [2]int32
	DifficultyMin, DifficultyMax uint8
	DistanceMin, DistanceMax     int32
	VillageDistance              int32
	WorldAgeMin, WorldAgeMax     int64

	DelayMin, DelayMax int32
	DelayID            string
	DelayChance        int32
	MobEvent           string
	Persistent         bool

	Herds         []Herd
	OnBlocks      []string
	NotOnBlocks   []string
	AboveBlocks   []string
	AboveDistance int32
	Weighted      []Permute
	Guaranteed    []Permute

	biome filter
	// slime marks minecraft:slime's rules, which BDS gives the slime
	// chunk predicate.
	slime bool
}

// defaultRule is a condition with no keys, as BDS's parser starts it.
func defaultRule(id string, pool Pool) Rule {
	return Rule{
		Identifier: id, Pool: pool, Weight: 100, AllowBubble: true,
		BrightnessMax: 15, MoonMax: 1,
		HeightMin: math.MinInt32, HeightMax: math.MaxInt32,
		Density:       [2]int32{-1, -1},
		DifficultyMax: 3, DistanceMin: 24, DistanceMax: 128,
		WorldAgeMax: -1, DelayChance: 100, AboveDistance: 1,
	}
}

// Rules are the loaded spawn rules, in load order.
type Rules struct {
	All []Rule
	// pools maps an entity identifier to its pool.
	pools map[string]Pool
}

// Pool returns the pool an entity identifier counts against.
func (r *Rules) Pool(identifier string) Pool {
	if p, ok := r.pools[identifier]; ok {
		return p
	}
	return PoolNone
}

type rawFile struct {
	Rules struct {
		Description struct {
			Identifier string `json:"identifier"`
			Population string `json:"population_control"`
		} `json:"description"`
		Conditions []map[string]json.RawMessage `json:"conditions"`
	} `json:"minecraft:spawn_rules"`
}

// Load reads helix-data's spawn_rules.json: an object of identifier to
// spawn rules file. Files are taken in identifier order.
func Load(data []byte) (*Rules, error) {
	var files map[string]rawFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("spawn: decode rules: %w", err)
	}
	ids := make([]string, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	rs := &Rules{pools: make(map[string]Pool, len(ids))}
	for _, id := range ids {
		f := files[id]
		pool := poolOf(f.Rules.Description.Population)
		if pool != PoolNone {
			rs.pools[f.Rules.Description.Identifier] = pool
		}
		for i, c := range f.Rules.Conditions {
			r, err := parseCondition(f.Rules.Description.Identifier, pool, c)
			if err != nil {
				return nil, fmt.Errorf("spawn: %s condition %d: %w", id, i, err)
			}
			rs.All = append(rs.All, r)
		}
	}
	return rs, nil
}

type minMax struct {
	Min *json.RawMessage `json:"min"`
	Max *json.RawMessage `json:"max"`
}

func intOr(m *json.RawMessage, def int64) int64 {
	if m == nil {
		return def
	}
	var f float64
	if json.Unmarshal(*m, &f) != nil {
		return def
	}
	return int64(f)
}

func difficulty(m *json.RawMessage, def uint8) uint8 {
	if m == nil {
		return def
	}
	var s string
	if json.Unmarshal(*m, &s) != nil {
		return def
	}
	switch s {
	case "peaceful":
		return 0
	case "easy":
		return 1
	case "normal":
		return 2
	case "hard":
		return 3
	}
	return def
}

type rawHerd struct {
	Min               *json.RawMessage `json:"min_size"`
	Max               *json.RawMessage `json:"max_size"`
	InitialEvent      string           `json:"initial_event"`
	InitialEventCount *json.RawMessage `json:"initial_event_count"`
	Event             string           `json:"event"`
	EventSkipCount    *json.RawMessage `json:"event_skip_count"`
}

type rawPermute struct {
	Weight     *json.RawMessage `json:"weight"`
	Guaranteed *json.RawMessage `json:"guaranteed_count"`
	Type       string           `json:"entity_type"`
}

// list decodes a value that may be one object or an array of them.
func list[T any](raw json.RawMessage) ([]T, error) {
	if s := strings.TrimSpace(string(raw)); strings.HasPrefix(s, "[") {
		var out []T
		err := json.Unmarshal(raw, &out)
		return out, err
	}
	var one T
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	return []T{one}, nil
}

// blockNames decodes a block filter: names, or objects with a name.
func blockNames(raw json.RawMessage) ([]string, error) {
	type named struct {
		Name string `json:"name"`
	}
	items, err := list[json.RawMessage](raw)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		var s string
		if json.Unmarshal(it, &s) == nil {
			out = append(out, s)
			continue
		}
		var n named
		if err := json.Unmarshal(it, &n); err != nil {
			return nil, err
		}
		out = append(out, n.Name)
	}
	return out, nil
}

func parseCondition(id string, pool Pool, c map[string]json.RawMessage) (Rule, error) {
	r := defaultRule(id, pool)
	r.slime = id == "minecraft:slime"
	if v, ok := c["minecraft:weight"]; ok {
		var w struct {
			Default *json.RawMessage `json:"default"`
			Rarity  *json.RawMessage `json:"rarity"`
		}
		if json.Unmarshal(v, &w) == nil {
			r.Weight = int32(max(intOr(w.Default, 100), 0))
			if w.Rarity != nil {
				r.Rarity = int32(intOr(w.Rarity, 5))
			}
		}
	}
	if v, ok := c["minecraft:density_limit"]; ok {
		var d struct {
			Surface     *json.RawMessage `json:"surface"`
			Underground *json.RawMessage `json:"underground"`
		}
		if json.Unmarshal(v, &d) == nil {
			r.Density = [2]int32{int32(intOr(d.Surface, -1)), int32(intOr(d.Underground, -1))}
		}
	}
	var err error
	if v, ok := c["minecraft:spawns_on_block_filter"]; ok {
		if r.OnBlocks, err = blockNames(v); err != nil {
			return r, fmt.Errorf("spawns_on_block_filter: %w", err)
		}
	}
	if v, ok := c["minecraft:spawns_on_block_prevented_filter"]; ok {
		if r.NotOnBlocks, err = blockNames(v); err != nil {
			return r, fmt.Errorf("spawns_on_block_prevented_filter: %w", err)
		}
	}
	if v, ok := c["minecraft:spawns_above_block_filter"]; ok {
		var a struct {
			Blocks   json.RawMessage  `json:"blocks"`
			Distance *json.RawMessage `json:"distance"`
		}
		if json.Unmarshal(v, &a) == nil {
			if len(a.Blocks) > 0 {
				if r.AboveBlocks, err = blockNames(a.Blocks); err != nil {
					return r, fmt.Errorf("spawns_above_block_filter: %w", err)
				}
			}
			r.AboveDistance = int32(intOr(a.Distance, 1))
		}
	}
	if v, ok := c["minecraft:herd"]; ok {
		hs, err := list[rawHerd](v)
		if err != nil {
			return r, fmt.Errorf("herd: %w", err)
		}
		for _, h := range hs {
			r.Herds = append(r.Herds, Herd{
				Min: int32(intOr(h.Min, 1)), Max: int32(intOr(h.Max, 1)),
				InitialEvent: h.InitialEvent, InitialEventCount: int32(intOr(h.InitialEventCount, 0)),
				Event: h.Event, EventSkipCount: int32(intOr(h.EventSkipCount, 0)),
			})
		}
	}
	if v, ok := c["minecraft:permute_type"]; ok {
		ps, err := list[rawPermute](v)
		if err != nil {
			return r, fmt.Errorf("permute_type: %w", err)
		}
		for _, p := range ps {
			e := Permute{Weight: int32(intOr(p.Weight, 1)), Guaranteed: int32(intOr(p.Guaranteed, 0)), Type: p.Type}
			if e.Guaranteed > 0 {
				r.Guaranteed = append(r.Guaranteed, e)
			}
			if e.Weight > 0 {
				r.Weighted = append(r.Weighted, e)
			}
		}
	}
	if v, ok := c["minecraft:brightness_filter"]; ok {
		var b struct {
			minMax
			Adjust bool `json:"adjust_for_weather"`
		}
		if json.Unmarshal(v, &b) == nil {
			r.BrightnessMin = int32(min(max(intOr(b.Min, 0), 0), 15))
			r.BrightnessMax = int32(min(max(intOr(b.Max, 15), 0), 15))
			r.AdjustForWeather = b.Adjust
		}
	}
	if v, ok := c["minecraft:height_filter"]; ok {
		var h minMax
		r.HeightMin, r.HeightMax = 24, 128
		if json.Unmarshal(v, &h) == nil {
			lo, hi := intOr(h.Min, 24), intOr(h.Max, 128)
			if lo > hi {
				lo, hi = hi, lo
			}
			r.HeightMin, r.HeightMax = int32(lo), int32(hi)
		}
	}
	_, r.Surface = c["minecraft:spawns_on_surface"]
	_, r.Underground = c["minecraft:spawns_underground"]
	_, r.Underwater = c["minecraft:spawns_underwater"]
	_, r.Lava = c["minecraft:spawns_lava"]
	if _, ok := c["minecraft:disallow_spawns_in_bubble"]; ok {
		r.AllowBubble = false
	}
	if v, ok := c["minecraft:biome_filter"]; ok {
		if r.biome, err = parseFilter(v); err != nil {
			return r, fmt.Errorf("biome_filter: %w", err)
		}
	}
	if v, ok := c["minecraft:difficulty_filter"]; ok {
		var d minMax
		if json.Unmarshal(v, &d) == nil {
			r.DifficultyMin, r.DifficultyMax = difficulty(d.Min, 0), difficulty(d.Max, 3)
		}
	}
	if v, ok := c["minecraft:distance_filter"]; ok {
		var d minMax
		lo, hi := int64(24), int64(128)
		if json.Unmarshal(v, &d) == nil {
			lo, hi = intOr(d.Min, 24), intOr(d.Max, 128)
			if lo > hi {
				lo, hi = hi, lo
			}
		}
		// BDS clamps the distances to [0, 128].
		r.DistanceMin, r.DistanceMax = int32(max(lo, 0)), int32(min(hi, 128))
	}
	if v, ok := c["minecraft:world_age_filter"]; ok {
		var a minMax
		lo, hi := int64(0), int64(-1)
		if json.Unmarshal(v, &a) == nil {
			lo, hi = intOr(a.Min, 0), intOr(a.Max, -1)
			if lo > hi && hi >= 0 {
				lo, hi = hi, lo
			}
		}
		r.WorldAgeMin, r.WorldAgeMax = lo, hi
	}
	if v, ok := c["minecraft:delay_filter"]; ok {
		var d struct {
			minMax
			Identifier string           `json:"identifier"`
			Chance     *json.RawMessage `json:"spawn_chance"`
		}
		if json.Unmarshal(v, &d) == nil {
			lo, hi := intOr(d.Min, 0), intOr(d.Max, 0)
			if lo > hi {
				lo, hi = hi, lo
			}
			r.DelayMin, r.DelayMax = int32(max(lo, 0)), int32(hi)
			r.DelayID = d.Identifier
			r.DelayChance = int32(intOr(d.Chance, 100))
		}
	}
	if v, ok := c["minecraft:mob_event_filter"]; ok {
		var e struct {
			Event string `json:"event"`
		}
		if json.Unmarshal(v, &e) == nil {
			r.MobEvent = e.Event
		}
	}
	_, r.Persistent = c["minecraft:is_persistent"]
	if v, ok := c["minecraft:player_in_village_filter"]; ok {
		var p struct {
			Distance *json.RawMessage `json:"distance"`
		}
		if json.Unmarshal(v, &p) == nil {
			r.VillageDistance = int32(intOr(p.Distance, 0))
		}
	}
	return r, nil
}
