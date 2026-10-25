// Package spawn is BDS's natural mob spawner: each tick it may pick a column
// in a ticking chunk, find the floors in it from the sky down, and spawn a
// herd on each floor from the spawn rules of that floor's biome.
//
// It follows BDS 1.26.52's BedrockSpawner. Not ported yet: structure spawn
// areas (witch huts, monuments, fortresses, outposts), slime chunks, phantom
// insomnia and despawning.
package spawn

import (
	"math"

	"github.com/helix-bedrock/entity/path"
)

// caps are the mob caps per dimension, [surface][pool], from BDS's Dimension
// constructors. A cap counts the mobs in the 9×9 chunks around the chunk
// that spawns.
var caps = [3][2][numPools]int32{
	Overworld: {
		{4, 16, 36, 0, 2, 0, 8}, // underground
		{4, 8, 36, 0, 0, 4, 8},  // surface
	},
	Nether: {
		{4, 16, 0, 0, 0, 0, 8},
		{0, 0, 0, 0, 0, 4, 8},
	},
	End: {
		{4, 16, 36, 0, 2, 0, 8},
		{4, 8, 36, 0, 0, 4, 8},
	},
}

// ramp is the dimension brightness ramp before its base is added: the
// brightness of each light level.
var ramp = [16]float32{0, 0.017543858, 0.037037037, 0.058823526, 0.083333328, 0.11111113,
	0.14285712, 0.1794872, 0.22222225, 0.27272731, 0.33333334, 0.40740743, 0.50000006,
	0.61904764, 0.77777773, 1}

// moon is the moon brightness of each moon phase.
var moon = [8]float32{1, 0.75, 0.5, 0.25, 0, 0.25, 0.5, 0.75}

// maxHerdTotal caps how many mobs one cluster may add.
const maxHerdTotal = 200

// Spawner holds the rules and the reusable counting state. One Spawner
// serves one dimension and must not be used from two goroutines at once.
type Spawner struct {
	rules  *Rules
	biomes []BiomeInfo
	// byBiome lists, per biome ID, the rules whose biome filter accepts the
	// biome, in load order.
	byBiome [][]int32
	// counts are the mobs near the chunk, [surface][pool].
	counts [2][numPools]int32
	// density counts the mobs near the chunk by identifier, [surface].
	density   [2]map[string]int32
	herdTotal int32
	// delays is the world age, in seconds, before which a delay_filter
	// identifier may not spawn again.
	delays  map[string]int64
	spawned []Mob
}

// New builds a Spawner. biomes is indexed by biome ID; an ID without an
// entry has no spawns.
func New(rules *Rules, biomes []BiomeInfo) *Spawner {
	s := &Spawner{
		rules:   rules,
		biomes:  biomes,
		byBiome: make([][]int32, len(biomes)),
		density: [2]map[string]int32{{}, {}},
		delays:  map[string]int64{},
	}
	for id := range biomes {
		for i := range rules.All {
			if rules.All[i].biome.test(&biomes[id]) {
				s.byBiome[id] = append(s.byBiome[id], int32(i))
			}
		}
	}
	for i := range rules.All {
		if d := rules.All[i].DelayID; d != "" {
			s.delays[d] = 0
		}
	}
	return s
}

// conditions is BDS's SpawnConditions for one floor.
type conditions struct {
	surface, water, bubble, lava, underground bool
	delay                                     int64
	brightness                                int32
	pos                                       path.Pos
}

func surfaceIndex(surface bool) int {
	if surface {
		return 1
	}
	return 0
}

// TickChunk runs the spawner for chunk (cx, cz), as BDS does once per tick
// for every ticking chunk.
func (s *Spawner) TickChunk(w World, lv *Level, r Rand, cx, cz int32) {
	if !lv.MobSpawning || r.Int(2000) > 10 {
		return
	}
	x := cx*16 + r.Int(16)
	z := cz*16 + r.Int(16)
	s.count(w, cx, cz)
	minY, maxY := w.HeightRange()
	p := path.Pos{X: x, Y: maxY, Z: z}
	found := findFloor(w, &p, true, minY)
	c := conditions{surface: true, pos: p}
	m1 := w.Block(path.Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Material
	m2 := w.Block(path.Pos{X: p.X, Y: p.Y + 2, Z: p.Z}).Material
	c.water = m1 == MaterialWater && m2 == MaterialWater
	c.lava = m1 == MaterialLava && m2 == MaterialLava
	if !w.Loaded(p) {
		return
	}
	if found && !w.StructureSpawns(p) && !w.InAncientCity(p) {
		s.cluster(w, lv, r, &c)
	}
	if w.Block(p).Material != MaterialLeaves {
		c.surface = false
	}
	c.underground = true
	c.delay = 0
	for findFloor(w, &p, false, minY) {
		if w.StructureSpawns(p) || w.InAncientCity(p) {
			continue
		}
		s.count(w, cx, cz)
		c.pos = p
		if w.Block(p).Material != MaterialLeaves {
			c.surface = false
		}
		m := w.Block(path.Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Material
		c.water = m == MaterialWater
		c.lava = m == MaterialLava
		s.cluster(w, lv, r, &c)
	}
}

// count recounts the mobs in the 9×9 chunks around (cx, cz).
func (s *Spawner) count(w World, cx, cz int32) {
	s.counts = [2][numPools]int32{}
	clear(s.density[0])
	clear(s.density[1])
	s.herdTotal = 0
	w.Mobs(cx-4, cz-4, cx+4, cz+4, func(id string, surface bool) {
		si := surfaceIndex(surface)
		if p := s.rules.Pool(id); p != PoolNone {
			s.counts[si][p]++
		}
		s.density[si][id]++
	})
}

// findFloor is BDS's findNextSpawnBlockUnder: it moves p down to the next
// block a mob may stand on. In surface mode a solid block on the way, or a
// carpet or bottom slab as the floor, ends the search.
func findFloor(w World, p *path.Pos, surface bool, minY int32) bool {
	for {
		if !w.Loaded(*p) {
			return false
		}
		if p.Y >= minY {
			// Leave the floor found last: down to air, a liquid or a bubble.
			for {
				b := w.Block(*p)
				if m := b.Material; m == MaterialAir || m == MaterialWater || m == MaterialLava || m == MaterialBubble {
					break
				}
				if surface && b.Flags&FlagSolid != 0 {
					return false
				}
				y := p.Y
				p.Y--
				if y <= minY {
					break
				}
			}
		}
		if p.Y < minY {
			return false
		}
		var b Block
		for {
			b = w.Block(*p)
			if b.Flags&(FlagCarpet|FlagBlocksMotion) != 0 {
				break
			}
			y := p.Y
			p.Y--
			if y <= minY {
				return false
			}
		}
		if b.Flags&FlagCarpet != 0 || b.Flags&(FlagSlab|FlagBottom) == FlagSlab|FlagBottom {
			if surface {
				return false
			}
			continue
		}
		if b.Material == MaterialIce || b.Flags&(FlagSolid|FlagSlab) != 0 {
			return true
		}
	}
}

// rawBrightness is BDS's getRawBrightness: the light at p with the sky light
// lowered by darken.
func rawBrightness(w World, p path.Pos, darken uint8) int32 {
	minY, maxY := w.HeightRange()
	if p.Y < minY {
		return 0
	}
	if p.Y >= maxY {
		return max(15-int32(darken), 0)
	}
	sky, block := w.Light(p)
	return max(int32(sky)-int32(darken), 0, int32(block))
}

// brightness is BDS's getBrightness: the ramp value of the raw brightness.
func brightness(w World, lv *Level, p path.Pos) float32 {
	base := float32(0)
	if lv.Dimension == Nether {
		base = 0.1
	}
	return min(max(ramp[rawBrightness(w, p, lv.SkyDarken)]+base, 0), 1)
}

func stormy(lv *Level) bool { return lv.Rain*lv.Thunder > 0.9 }

func (s *Spawner) cluster(w World, lv *Level, r Rand, c *conditions) {
	if w.Block(c.pos).Material == MaterialAir {
		return
	}
	up := path.Pos{X: c.pos.X, Y: c.pos.Y + 1, Z: c.pos.Z}
	c.brightness = rawBrightness(w, up, lv.SkyDarken)
	pick := *c
	if c.water || c.lava {
		pick.pos = up
	}
	rule := s.pick(w, lv, r, &pick)
	if rule == nil {
		return
	}
	herd := Herd{Min: 1, Max: 1}
	if n := int32(len(rule.Herds)); n > 0 {
		herd = rule.Herds[r.Int(n)]
	}
	c.delay = 0
	if rule.DelayID != "" {
		c.delay = s.delays[rule.DelayID]
	}
	if !s.passes(w, lv, r, rule, c) || rule.Rarity > 0 && r.Int(rule.Rarity) != 0 {
		return
	}
	f := r.Float()
	n := herd.Min + int32(math.Round(float64(f*f*float32(herd.Max-herd.Min))))
	if n <= 0 || !s.placement(w, rule, up) {
		return
	}
	if c.delay != 0 && rule.DelayChance != 100 && r.Int(100) > rule.DelayChance {
		s.setDelay(lv, r, rule)
		return
	}
	if s.herdTotal+n >= maxHerdTotal+1 {
		n = max(maxHerdTotal-s.herdTotal, 0)
	}
	si := surfaceIndex(c.surface)
	if d := rule.Density[1-si]; d >= 0 {
		if cur := s.density[si][rule.Identifier]; cur+n >= d {
			n = d - cur
		}
	}
	s.spawned = s.spawned[:0]
	typ := rule.Identifier
	for i := 0; i < len(rule.Guaranteed) && n > 0; i++ {
		g := &rule.Guaranteed[i]
		if g.Type == "" {
			continue
		}
		typ = g.Type
		tries := min(g.Guaranteed, n)
		for range tries {
			if s.capOK(lv, rule, si) {
				s.spawnOne(w, lv, r, typ, c)
			}
		}
		n -= tries
	}
	for ; n > 0; n-- {
		if !s.capOK(lv, rule, si) {
			continue
		}
		var total int32
		for i := range rule.Weighted {
			total += rule.Weighted[i].Weight
		}
		if len(rule.Weighted) > 0 {
			var roll int32
			if total != 0 {
				roll = r.Int(total)
			}
			for i := range rule.Weighted {
				if roll -= rule.Weighted[i].Weight; roll < 0 {
					if t := rule.Weighted[i].Type; t != "" {
						typ = t
					}
					break
				}
			}
		}
		s.spawnOne(w, lv, r, typ, c)
	}
	if c.delay != 0 && len(s.spawned) > 0 {
		s.setDelay(lv, r, rule)
	}
	if herd.InitialEvent != "" || herd.Event != "" {
		for i, m := range s.spawned {
			if int32(i) < herd.InitialEventCount && herd.InitialEvent != "" {
				m.Event(herd.InitialEvent)
			}
			if int32(i) >= herd.EventSkipCount && herd.Event != "" {
				m.Event(herd.Event)
			}
		}
	}
	if rule.Persistent {
		for _, m := range s.spawned {
			m.Persist()
		}
	}
}

func (s *Spawner) setDelay(lv *Level, r Rand, rule *Rule) {
	if _, ok := s.delays[rule.DelayID]; !ok {
		return
	}
	d := rule.DelayMin
	if rule.DelayMax > rule.DelayMin {
		d += r.Int(rule.DelayMax - rule.DelayMin)
	}
	s.delays[rule.DelayID] = int64(d) + lv.Time/20
}

func (s *Spawner) capOK(lv *Level, rule *Rule, si int) bool {
	if rule.Pool == PoolNone || int(lv.Dimension) >= len(caps) {
		return false
	}
	limit := caps[lv.Dimension][si][rule.Pool]
	return limit > 0 && s.counts[si][rule.Pool] < limit
}

// spawnOne is BDS's spawn of one mob of a cluster, standing on the floor.
func (s *Spawner) spawnOne(w World, lv *Level, r Rand, typ string, c *conditions) {
	pos := path.Vec3{X: float32(c.pos.X) + 0.49, Y: float32(c.pos.Y + 1), Z: float32(c.pos.Z) + 0.49}
	m := w.NewMob(typ, pos, c.surface)
	if m == nil || !checkSpawn(w, lv, r, typ, m) {
		return
	}
	m.Add()
	s.spawned = append(s.spawned, m)
	si := surfaceIndex(c.surface)
	if p := s.rules.Pool(typ); p != PoolNone {
		s.counts[si][p]++
	}
	s.density[si][typ]++
}

// pick is BDS's getMobToSpawn: a weighted pick among the biome's rules that
// pass the conditions.
func (s *Spawner) pick(w World, lv *Level, r Rand, c *conditions) *Rule {
	id := w.Biome(c.pos)
	if int(id) >= len(s.byBiome) {
		return nil
	}
	list := s.byBiome[id]
	var total int32
	for _, i := range list {
		if rule := &s.rules.All[i]; s.passes(w, lv, r, rule, c) {
			total += rule.Weight
		}
	}
	var roll int32
	if total != 0 {
		roll = r.Int(total)
	}
	for _, i := range list {
		rule := &s.rules.All[i]
		if !s.passes(w, lv, r, rule, c) {
			continue
		}
		if roll -= rule.Weight; roll < 0 {
			return rule
		}
	}
	return nil
}

// moonPhase is BDS's moon phase of a time of day.
func moonPhase(t int64) int {
	d := t / -24000
	if d < 0 {
		d = t / 24000
	}
	return int((d ^ (t >> 63)) & 7)
}

// passes is BDS's canSpawnInConditions.
func (s *Spawner) passes(w World, lv *Level, r Rand, rule *Rule, c *conditions) bool {
	minY, maxY := w.HeightRange()
	y := c.pos.Y
	if y < minY || y >= maxY || y < rule.HeightMin || y > rule.HeightMax {
		return false
	}
	if c.water && !rule.Underwater || c.bubble && !rule.AllowBubble || c.lava && !rule.Lava ||
		c.surface && !rule.Surface || c.underground && !rule.Underground {
		return false
	}
	if rule.slime && !s.slimeOK(w, lv, r, c) {
		return false
	}
	if lv.Difficulty < rule.DifficultyMin || lv.Difficulty > rule.DifficultyMax {
		return false
	}
	b := c.brightness
	if rule.AdjustForWeather && stormy(lv) {
		b = rawBrightness(w, c.pos, 10)
	}
	if b < rule.BrightnessMin || b > rule.BrightnessMax {
		return false
	}
	if mb := moon[moonPhase(lv.DayTime)]; mb < rule.MoonMin || mb > rule.MoonMax {
		return false
	}
	// BDS compares the ages unsigned, so a max of −1 has no limit.
	age := uint64(lv.Time / 20)
	if age < uint64(rule.WorldAgeMin) || age > uint64(rule.WorldAgeMax) || age <= uint64(c.delay) {
		return false
	}
	if rule.MobEvent != "" && !w.MobEvent(rule.MobEvent) {
		return false
	}
	return rule.VillageDistance == 0 || !w.PlayerInVillage(c.pos, rule.VillageDistance)
}

// placement is BDS's spawn position check at p, the block above the floor.
func (s *Spawner) placement(w World, rule *Rule, p path.Pos) bool {
	cx, cz := p.X>>4, p.Z>>4
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			if !w.TickedRecently(cx+dx, cz+dz) {
				return false
			}
		}
	}
	maxDist := float32(math.MaxFloat32)
	if rule.DistanceMax >= 0 {
		maxDist = float32(rule.DistanceMax)
	}
	pv := path.Vec3{X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z)}
	pl, ok := w.NearestPlayer(pv, maxDist)
	if !ok {
		return false
	}
	dx, dy, dz := pv.X-pl.X, pv.Y-pl.Y, pv.Z-pl.Z
	if float32(rule.DistanceMin*rule.DistanceMin) > dx*dx+dy*dy+dz*dz {
		return false
	}
	below := w.Block(path.Pos{X: p.X, Y: p.Y - 1, Z: p.Z})
	if len(rule.OnBlocks) > 0 && !contains(rule.OnBlocks, below.Name) || contains(rule.NotOnBlocks, below.Name) {
		return false
	}
	if len(rule.AboveBlocks) > 0 && !above(w, rule, p) {
		return false
	}
	at := w.Block(p)
	if w.Block(path.Pos{X: p.X, Y: p.Y + 1, Z: p.Z}).Flags&FlagSolidBlocking != 0 {
		return false
	}
	if rule.Underwater != (at.Material == MaterialWater) || rule.Lava != (at.Material == MaterialLava) {
		return false
	}
	if below.Flags&FlagNoSpawnBelow != 0 {
		return false
	}
	return at.Flags&FlagSolidBlocking == 0
}

// above is spawns_above_block_filter: one of the blocks within the distance
// below p, with only water or air between.
func above(w World, rule *Rule, p path.Pos) bool {
	for i := int32(1); i <= rule.AboveDistance; i++ {
		b := w.Block(path.Pos{X: p.X, Y: p.Y - i, Z: p.Z})
		if contains(rule.AboveBlocks, b.Name) {
			return true
		}
		if b.Material != MaterialWater && b.Material != MaterialAir {
			return false
		}
	}
	return false
}

func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// slimeOK is the spawn predicate BDS gives minecraft:slime's rules: slime
// chunks below y 39, and swamps on dim moonlit nights from y 50 to 68.
func (s *Spawner) slimeOK(w World, lv *Level, r Rand, c *conditions) bool {
	p := c.pos
	if p.Y <= 38 {
		return slimeChunk(p.X>>4, p.Z>>4)
	}
	if p.Y < 50 || p.Y > 68 || c.brightness > 7 || c.brightness > r.Int(8) {
		return false
	}
	if moon[moonPhase(lv.DayTime)] <= r.Float() {
		return false
	}
	id := w.Biome(path.Pos{X: p.X, Y: p.Y + 1, Z: p.Z})
	return int(id) < len(s.biomes) && contains(s.biomes[id].Tags, "spawns_slimes_on_surface")
}

// slimeChunk reports whether a chunk is a slime chunk: the first output of
// MT19937 seeded with cz ^ cx*522133279 is a multiple of 10.
func slimeChunk(cx, cz int32) bool {
	var mt [398]uint32
	mt[0] = uint32(cz) ^ uint32(cx)*522133279
	for i := 1; i < len(mt); i++ {
		mt[i] = 1812433253*(mt[i-1]^mt[i-1]>>30) + uint32(i)
	}
	y := mt[0]&0x80000000 | mt[1]&0x7fffffff
	v := mt[397] ^ y>>1
	if y&1 != 0 {
		v ^= 0x9908b0df
	}
	v ^= v >> 11
	v ^= v << 7 & 0x9d2c5680
	v ^= v << 15 & 0xefc60000
	v ^= v >> 18
	return v%10 == 0
}
