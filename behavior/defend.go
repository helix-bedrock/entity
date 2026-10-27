package behavior

import "math"

// NewNearestPrioritized makes NearestPrioritizedAttackableTargetGoal:
// nearest_attackable_target that picks, among the entities that pass, the
// one whose entity_types entry has the lowest priority (the nearest on
// ties), and only turns set_persistent on for players.
func NewNearestPrioritized(w World, m Targeter, c NearestAttackableConfig) *NearestAttackable {
	g := NewNearestAttackable(w, m, c)
	g.prioritized = true
	return g
}

// Truster is a mob with TrustComponent.
type Truster interface {
	Targeter
	// Trusted are the IDs the mob trusts, in BDS's set order; nil when the
	// mob has no TrustComponent.
	Trusted() []ID
}

// DefendTrustedConfig is the JSON of minecraft:behavior.defend_trusted_target:
// nearest_attackable_target's fields plus the defend start.
type DefendTrustedConfig struct {
	NearestAttackableConfig
	AggroSound    int `json:"aggro_sound"`
	OnDefendStart string
}

// DefaultDefendTrusted returns the goal's BDS defaults.
func DefaultDefendTrusted() DefendTrustedConfig {
	return DefendTrustedConfig{NearestAttackableConfig: DefaultNearestAttackable(), AggroSound: NoSound}
}

// DefendTrusted is DefendTrustedTargetGoal: the mob targets whatever last
// hurt one of the entities it trusts.
type DefendTrusted struct {
	*NearestAttackable
	t    Truster
	dcfg DefendTrustedConfig
}

// NewDefendTrusted makes the goal.
func NewDefendTrusted(w World, t Truster, c DefendTrustedConfig) *DefendTrusted {
	return &DefendTrusted{NearestAttackable: NewNearestAttackable(w, t, c.NearestAttackableConfig), t: t, dcfg: c}
}

// CanUse implements goal.Goal (0xBDD4F40).
func (g *DefendTrusted) CanUse() bool {
	if g.t.Trusted() == nil || !g.due() || g.actor(g.t.Target()) != nil {
		return false
	}
	g.candidate = g.attacker()
	return g.candidate != NoID
}

// attacker is the nearest entity that hurt a trusted entity (0xBDD51B0).
// An attacker beyond the follow range clears the mob's target.
func (g *DefendTrusted) attacker() ID {
	trusted := g.t.Trusted()
	r := g.t.FollowRange()
	best, bestD := NoID, float32(math.MaxFloat32)
	for _, id := range trusted {
		f := g.actor(id)
		if f == nil || f.Removed() {
			continue
		}
		a := g.actor(f.LastHurtByMob())
		if a == nil {
			continue
		}
		d := dist2(g.t.Pos(), a.Pos())
		if d > r*r {
			g.t.SetTarget(NoID)
			continue
		}
		if contains(trusted, a.ID()) {
			continue
		}
		if d < bestD {
			best, bestD = a.ID(), d
		}
	}
	return best
}

// Start implements goal.Goal (0xBDD5C20).
func (g *DefendTrusted) Start() {
	if a := g.actor(g.candidate); a != nil {
		g.t.SetTarget(a.ID())
	}
	if g.dcfg.AggroSound != NoSound {
		g.t.PlaySound(g.dcfg.AggroSound)
	}
	if g.dcfg.OnDefendStart != "" {
		g.t.Event(g.dcfg.OnDefendStart, NoID)
	}
}

// VillageDefender is a mob that defends its village (DwellerComponent).
type VillageDefender interface {
	Targeter
	// VillageAggressor is the village's aggressor nearest the mob, NoID for
	// none; ok is false when the mob has no village.
	VillageAggressor() (id ID, ok bool)
	// VillageBadPlayer is the nearest player whose standing with the
	// village is -15 or lower, NoID for none.
	VillageBadPlayer() ID
}

// DefendVillageConfig is the JSON of minecraft:behavior.defend_village_target.
type DefendVillageConfig struct {
	NearestAttackableConfig
	AttackChance float32 `json:"attack_chance"`
}

// DefaultDefendVillage returns the goal's BDS defaults.
func DefaultDefendVillage() DefendVillageConfig {
	return DefendVillageConfig{NearestAttackableConfig: DefaultNearestAttackable(), AttackChance: 0.05}
}

// DefendVillage is DefendVillageTargetGoal: the mob now and then targets
// the village's nearest aggressor, or a player the village dislikes.
type DefendVillage struct {
	*NearestAttackable
	v    VillageDefender
	dcfg DefendVillageConfig
}

// NewDefendVillage makes the goal.
func NewDefendVillage(w World, v VillageDefender, c DefendVillageConfig) *DefendVillage {
	return &DefendVillage{NearestAttackable: NewNearestAttackable(w, v, c.NearestAttackableConfig), v: v, dcfg: c}
}

// valid is TargetGoal's target test (0xBDD60E0): canAttack, an entity_types
// entry when there are any (else must_see on its own), and must_reach.
func (g *DefendVillage) valid(a Actor) bool {
	if !g.canAttack(a) {
		return false
	}
	if len(g.cfg.EntityTypes) != 0 {
		if g.match(a) < 0 {
			return false
		}
	} else if g.mustSee && !(a.ID() == g.v.Target() && g.unseen < g.forget) && !g.v.CanSee(a) {
		return false
	}
	return !g.mustReach || g.canReach(a)
}

// CanUse implements goal.Goal (0x576C340). As in BDS, an aggressor whose
// filters fail is still tested, but start then clears the target.
func (g *DefendVillage) CanUse() bool {
	id, ok := g.v.VillageAggressor()
	if !ok {
		return false
	}
	a := g.actor(id)
	if g.dcfg.AttackChance <= g.world.Random().NextFloat() {
		return false
	}
	if a != nil && a.IsPlayer() {
		a = g.actor(g.v.VillageBadPlayer())
	}
	g.candidate = NoID
	if a == nil {
		return false
	}
	if allFilters(g.cfg.EntityTypes, g.v, a) {
		g.candidate = a.ID()
	}
	return g.valid(a)
}

// CanContinueToUse implements goal.Goal (0x576C6B0).
func (g *DefendVillage) CanContinueToUse() bool {
	c := g.actor(g.candidate)
	if c == nil || c.Removed() {
		g.keep(g.actor(g.v.Target()))
		return false
	}
	if !g.keep(g.actor(g.v.Target())) {
		return false
	}
	return !g.mustReach || g.canReach(c)
}

// Start implements goal.Goal (0x576C8F0).
func (g *DefendVillage) Start() {
	target := NoID
	if c := g.actor(g.candidate); c != nil && !c.Removed() {
		target = c.ID()
	}
	g.v.SetTarget(target)
	g.reached, g.unseen = reachUnknown, 0
}
