package behavior

import "strings"

// The actor categories (BDS's ActorCategory bits) attack_types names.
const (
	CategoryPlayer   uint32 = 1
	CategoryMob      uint32 = 2
	CategoryMonster  uint32 = 4
	CategoryHumanoid uint32 = 8
	CategoryAnimal   uint32 = 16
	CategoryWater    uint32 = 32
	CategoryPathable uint32 = 64
	CategoryTamable  uint32 = 128
	CategoryRidable  uint32 = 256
	CategoryItem     uint32 = 1024
	CategoryAmbient  uint32 = 2048
	CategoryVillager uint32 = 4096
	// categoryMonsterClass is what BDS's Monster constructor adds
	// (1.26.60): mob, monster and one more bit no name reaches.
	categoryMonsterClass uint32 = 0x80006
)

// categoryNames is BDS 1.26.60's name table, which also has
// names for unions of categories.
func categoryNames(name string) uint32 {
	switch name {
	case "player":
		return CategoryPlayer
	case "mob":
		return CategoryMob
	case "monster":
		return CategoryMonster
	case "humanoid":
		return CategoryHumanoid
	case "animal":
		return CategoryAnimal
	case "water":
		return CategoryWater
	case "pathable":
		return CategoryPathable
	case "tamable":
		return CategoryTamable
	case "ridable":
		return CategoryRidable
	case "item":
		return CategoryItem
	case "ambient":
		return CategoryAmbient
	case "villager":
		return CategoryVillager
	case "boatrideable":
		return 131328
	case "humanoidmonster":
		return 12
	case "wateranimal":
		return 48
	case "tamableanimal":
		return 144
	case "undeadmob", "undeadmonster":
		return 16388
	case "zombiemonster":
		return 32772
	}
	return 0
}

// ActorCategories reads an attack_types string as BDS does: the
// names split at spaces, tabs and NULs, each looked up without case, ORed.
// "none" and unknown names add nothing.
func ActorCategories(s string) uint32 {
	var c uint32
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == 0 }) {
		c |= categoryNames(strings.ToLower(f))
	}
	return c
}

// Categorized is an entity that knows its BDS actor categories (Actor),
// which BDS sets from the entity's C++ class.
type Categorized interface {
	Categories() uint32
}

// ActorCategoriesOf is the entity's categories. Without Categorized it is
// what the player and monster classes' constructors set; other classes'
// categories are not known then.
func ActorCategoriesOf(a Actor) uint32 {
	if c, ok := a.(Categorized); ok {
		return c.Categories()
	}
	switch {
	case a.IsPlayer():
		return CategoryPlayer | CategoryMob
	case a.Monster():
		return categoryMonsterClass
	}
	return 0
}
