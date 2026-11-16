package spawn

import (
	"math"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/path"
)

// class is the BDS actor class a mob is made from, which decides its own
// spawn check (Mob::checkSpawnRules and its overrides).
type class uint8

const (
	classMob class = iota
	classMonster
	// classDarkMonster is a Monster whose isDarkEnoughToSpawn is always
	// true: piglins, blazes, ghasts, pillagers, guardians.
	classDarkMonster
	classAnimal
	classWater
	classGuardian
	classPigZombie
	classMushroomCow
	classSlime
	classMagmaCube
	classSilverfish
	classEndermite
	classPhantom
	// classAlways always passes: warden, creaking, sulfur cube.
	classAlways
	classStrider
	classParrot
	classTurtle
	// classZeroWalkMonster is a Monster whose walk target is 0
	// (vindicators).
	classZeroWalkMonster
	// classZeroWalkDark is a Monster that is always dark enough and whose
	// walk target is 0 (ghasts, pillagers).
	classZeroWalkDark
	// classAxolotl is an Animal whose walk target is 0.
	classAxolotl
)

// classOf maps identifiers to their class, from BDS's ActorFactory. Mobs
// not listed are plain Mobs.
func classOf(id string) class {
	switch id {
	case "minecraft:zombie", "minecraft:husk", "minecraft:drowned", "minecraft:zombie_villager",
		"minecraft:zombie_villager_v2", "minecraft:skeleton", "minecraft:stray",
		"minecraft:wither_skeleton", "minecraft:bogged", "minecraft:parched", "minecraft:spider",
		"minecraft:cave_spider", "minecraft:creeper", "minecraft:enderman", "minecraft:witch",
		"minecraft:breeze", "minecraft:zoglin", "minecraft:evocation_illager", "minecraft:vex",
		"minecraft:ravager", "minecraft:wither", "minecraft:ender_dragon":
		return classMonster
	case "minecraft:vindicator":
		return classZeroWalkMonster
	case "minecraft:piglin", "minecraft:piglin_brute", "minecraft:blaze":
		return classDarkMonster
	case "minecraft:ghast", "minecraft:pillager":
		return classZeroWalkDark
	case "minecraft:guardian", "minecraft:elder_guardian":
		return classGuardian
	case "minecraft:zombie_pigman":
		return classPigZombie
	case "minecraft:slime":
		return classSlime
	case "minecraft:magma_cube":
		return classMagmaCube
	case "minecraft:silverfish":
		return classSilverfish
	case "minecraft:endermite":
		return classEndermite
	case "minecraft:phantom":
		return classPhantom
	case "minecraft:warden", "minecraft:creaking", "minecraft:sulfur_cube":
		return classAlways
	case "minecraft:mooshroom":
		return classMushroomCow
	case "minecraft:strider":
		return classStrider
	case "minecraft:parrot":
		return classParrot
	case "minecraft:turtle":
		return classTurtle
	case "minecraft:cow", "minecraft:chicken", "minecraft:pig", "minecraft:sheep", "minecraft:wolf",
		"minecraft:rabbit", "minecraft:ocelot", "minecraft:cat", "minecraft:llama",
		"minecraft:trader_llama", "minecraft:polar_bear", "minecraft:panda", "minecraft:bee",
		"minecraft:goat", "minecraft:camel", "minecraft:camel_husk", "minecraft:sniffer",
		"minecraft:armadillo", "minecraft:horse", "minecraft:donkey", "minecraft:mule",
		"minecraft:skeleton_horse", "minecraft:zombie_horse", "minecraft:fox", "minecraft:frog",
		"minecraft:happy_ghast":
		return classAnimal
	case "minecraft:axolotl":
		return classAxolotl
	case "minecraft:dolphin", "minecraft:salmon", "minecraft:pufferfish", "minecraft:cod",
		"minecraft:tropicalfish", "minecraft:tadpole", "minecraft:squid", "minecraft:glow_squid",
		"minecraft:nautilus", "minecraft:zombie_nautilus":
		return classWater
	}
	return classMob
}

func floorPos(v path.Vec3) path.Pos {
	return path.Pos{X: int32(math.Floor(float64(v.X))), Y: int32(math.Floor(float64(v.Y))), Z: int32(math.Floor(float64(v.Z)))}
}

func below(p path.Pos) path.Pos { return path.Pos{X: p.X, Y: p.Y - 1, Z: p.Z} }

// CheckSpawnRules is the mob's own spawn check (Mob::checkSpawnRules and its overrides),
// which BDS also runs for a monster spawner's mobs: Spawner::spawnMob (1.26.60 0xCD8F8C0)
// asks it of every mob spawned with its natural spawn flag set, and the spawner's flag
// changes nothing in Mob's (0xBFE5530) or Monster's (0xBD3E490, darkness first) checks.
func CheckSpawnRules(w World, lv *Level, r Rand, typ string, m Mob) bool {
	return checkSpawn(w, lv, r, typ, m)
}

// checkSpawn is the mob's own natural spawn check, run before it is added.
func checkSpawn(w World, lv *Level, r Rand, typ string, m Mob) bool {
	c := classOf(typ)
	switch c {
	case classAlways:
		return true
	case classWater:
		return waterCheck(w, r, m)
	case classGuardian:
		if r.Int(20) == 0 {
			return true
		}
		// BDS asks whether the position sees the sky.
		sky, _ := w.Light(floorPos(m.Pos()))
		return sky < 15
	case classPigZombie:
		return w.EntitiesIn(m.Box()) == 0
	case classMushroomCow:
		p := feet(m)
		return w.Block(below(p)).Flags&FlagMycelium != 0 && mobCheck(w, lv, typ, m)
	case classPhantom:
		// Phantoms come from insomnia, not from the natural spawner.
		return false
	case classSilverfish:
		if _, near := w.NearestPlayer(m.Pos(), 5); near {
			return false
		}
		return mobCheck(w, lv, typ, m)
	case classEndermite:
		if !darkEnough(w, lv, c, m) || !mobCheck(w, lv, typ, m) {
			return false
		}
		_, near := w.NearestPlayer(m.Pos(), 5)
		return !near
	}
	if !darkEnough(w, lv, c, m) {
		return false
	}
	return mobCheck(w, lv, typ, m)
}

func isMonster(c class) bool {
	switch c {
	case classMonster, classDarkMonster, classZeroWalkMonster, classZeroWalkDark, classMagmaCube, classEndermite:
		return true
	}
	return false
}

// darkEnough is Monster::isDarkEnoughToSpawn; other classes pass.
func darkEnough(w World, lv *Level, c class, m Mob) bool {
	if c == classDarkMonster || c == classZeroWalkDark || c == classMagmaCube {
		return true
	}
	if !isMonster(c) {
		return true
	}
	p := floorPos(m.Pos())
	if _, block := w.Light(p); block > 0 {
		return false
	}
	raw := rawBrightness(w, p, lv.SkyDarken)
	if stormy(lv) {
		raw = rawBrightness(w, p, 10)
	}
	return raw < 8
}

// feet is the block position BDS's Mob check uses: the mob's x and z with
// the bottom of its box.
func feet(m Mob) path.Pos {
	v := m.Pos()
	v.Y = m.Box().Min.Y
	return floorPos(v)
}

// mobCheck is Mob::checkSpawnRules.
func mobCheck(w World, lv *Level, typ string, m Mob) bool {
	p := feet(m)
	if solidIn(w, m.Box()) && w.EntitiesIn(m.Box()) > 0 {
		return false
	}
	if !m.FireImmune() {
		if f := w.Block(below(p)).Flags; f&(FlagMagma|FlagCampfire) != 0 {
			return false
		}
	}
	return walkTarget(w, lv, typ, m, p) >= 0
}

// walkWorld is the spawner's world as the walk target scores read it.
type walkWorld struct {
	w  World
	lv *Level
}

func (w walkWorld) Block(p path.Pos) behavior.Block {
	b := w.w.Block(p)
	return behavior.Block{Name: b.Name, Material: path.Material(b.Material)}
}

func (w walkWorld) Brightness(p path.Pos) float32 { return brightness(w.w, w.lv, p) }

// walkTarget is the mob's getWalkTargetValue at p (behavior.WalkTarget).
func walkTarget(w World, lv *Level, typ string, m Mob, p path.Pos) float32 {
	b := behavior.DefaultWalkBlocks()
	return behavior.WalkTarget(behavior.WalkKindOf(typ), &b, walkWorld{w, lv}, p, m.FireImmune())
}

// waterCheck is WaterAnimal::checkSpawnRules: the mob is moved down a random
// amount below the top of the water column, then needs water around it.
func waterCheck(w World, r Rand, m Mob) bool {
	pos := m.Pos()
	p := floorPos(pos)
	top := p.Y
	for w.Block(path.Pos{X: p.X, Y: top + 1, Z: p.Z}).Material == MaterialWater {
		top++
	}
	y := max(top-r.Int(4)-2, int32(pos.Y))
	m.SetPos(path.Vec3{X: float32(p.X), Y: float32(y), Z: float32(p.Z)})
	b := m.Box()
	if solidIn(w, b) || !materialIn(w, b, MaterialWater) {
		return false
	}
	return w.Block(path.Pos{X: p.X, Y: y, Z: p.Z}).Flags&FlagSource != 0
}

// solidIn reports whether a block that blocks motion and is not translucent
// overlaps b.
func solidIn(w World, b path.Box) bool {
	x0, x1 := ffloor(b.Min.X), fceil(b.Max.X)
	y0, y1 := ffloor(b.Min.Y), fceil(b.Max.Y)
	z0, z1 := ffloor(b.Min.Z), fceil(b.Max.Z)
	if x0 == x1 || y0 == y1 || z0 == z1 {
		return false
	}
	for x := x0; x < x1; x++ {
		for y := y0; y < y1; y++ {
			for z := z0; z < z1; z++ {
				f := w.Block(path.Pos{X: x, Y: y, Z: z}).Flags
				if f&FlagTranslucent == 0 && f&FlagBlocksMotion != 0 {
					return true
				}
			}
		}
	}
	return false
}

// materialIn reports whether a block of material m is in the blocks b
// touches.
func materialIn(w World, b path.Box, m Material) bool {
	for x := ffloor(b.Min.X); x <= ffloor(b.Max.X); x++ {
		for y := ffloor(b.Min.Y); y <= ffloor(b.Max.Y); y++ {
			for z := ffloor(b.Min.Z); z <= ffloor(b.Max.Z); z++ {
				if w.Block(path.Pos{X: x, Y: y, Z: z}).Material == m {
					return true
				}
			}
		}
	}
	return false
}

func ffloor(f float32) int32 { return int32(math.Floor(float64(f))) }
func fceil(f float32) int32  { return int32(math.Ceil(float64(f))) }
