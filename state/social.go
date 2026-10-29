package state

import (
	"sort"

	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// TrustingConfig is minecraft:trusting (1.21.3 schema 0x14062F2C0).
type TrustingConfig struct {
	Probability float32
	Items       []string
	OnTrust     *definition.Trigger
	// InteractText is the interaction button's text.
	InteractText string
	// TrustEvent is the actor event broadcast when the mob trusts.
	TrustEvent int
}

// DefaultTrusting is the BDS default.
func DefaultTrusting() TrustingConfig {
	return TrustingConfig{InteractText: "action.interact.feed", TrustEvent: EventTameSuccess}
}

// TrustingInteraction is TrustingComponent::getInteraction (1.21.3
// 0x140637650): a mob that does not trust yet offers feeding when the
// hand holds a trust item. It returns the interaction text, "" for none.
func TrustingInteraction(m Mob, c *TrustingConfig, hand Hand) string {
	if m.Flag(FlagTrusting) {
		return ""
	}
	for _, it := range c.Items {
		if hand != nil && hand.Matches(it) {
			return c.InteractText
		}
	}
	return ""
}

// TryTrust is the feeding: with the probability the mob becomes trusting.
// The roll lives in a captured BDS lambda that was not read; the
// probability member says what it does.
func TryTrust(env *Env, c *TrustingConfig) bool {
	if env.World.Random().NextFloat() >= c.Probability {
		return false
	}
	BecomeTrusting(env, c)
	return true
}

// BecomeTrusting is TrustingComponent::_becomeTrusting (0x1406240D0): the
// mob turns persistent, broadcasts the event, raises the trusting flag
// and runs trust_event.
func BecomeTrusting(env *Env, c *TrustingConfig) {
	m := env.Self
	if p, ok := m.(interface{ SetPersistent() }); ok {
		p.SetPersistent()
	}
	m.ActorEvent(c.TrustEvent)
	m.SetFlag(FlagTrusting, true)
	env.fire(c.OnTrust, env.tester(m, nil))
}

// AlleleRange is an inclusive allele range; -1 bounds mean unset.
type AlleleRange struct{ Min, Max int32 }

func (r AlleleRange) set() bool        { return r.Min >= 0 && r.Max >= 0 }
func (r AlleleRange) has(v int32) bool { return v >= r.Min && v <= r.Max }
func (r AlleleRange) draw(rnd behavior.Random) int32 {
	if r.Max <= r.Min || rnd == nil {
		return r.Min
	}
	return r.Min + rnd.NextInt(r.Max-r.Min+1)
}

// GeneticVariant is one of a gene's genetic_variants: the birth event run
// when the alleles fit every range it sets.
type GeneticVariant struct {
	Main, Hidden, Either, Both AlleleRange
	Birth                      *definition.Trigger
}

// Gene is one of minecraft:genetics' genes.
type Gene struct {
	Name     string
	Alleles  AlleleRange
	Variants []GeneticVariant
	// Simplified is use_simplified_breeding.
	Simplified bool
	// MutationRate overrides the component's when not negative.
	MutationRate float32
}

// GeneticsConfig is minecraft:genetics (1.21.3 schema 0x1405AC0A0).
type GeneticsConfig struct {
	MutationRate float32
	Genes        []Gene
}

// Alleles is a gene's main and hidden allele.
type Alleles struct{ Main, Hidden int32 }

// Genetics is the running component (1.21.3 initialize 0x1405B46E0,
// initializeGenesFromParents 0x1405B4AD0, _crossParentGenes 0x1405A0BE0,
// fireGeneEvents 0x1405B0250).
type Genetics struct {
	cfg   GeneticsConfig
	genes []Alleles
}

// NewGenetics makes the component with random alleles for each gene.
func NewGenetics(c GeneticsConfig, r behavior.Random) *Genetics {
	g := &Genetics{cfg: c, genes: make([]Alleles, len(c.Genes))}
	for i, gene := range c.Genes {
		g.genes[i] = Alleles{gene.Alleles.draw(r), gene.Alleles.draw(r)}
	}
	return g
}

// Config returns the settings.
func (g *Genetics) Config() GeneticsConfig { return g.cfg }

// SetConfig replaces the settings.
func (g *Genetics) SetConfig(c GeneticsConfig) { g.cfg = c }

// Genes are the alleles by gene, in the config's order.
func (g *Genetics) Genes() []Alleles { return g.genes }

// SetGenes restores saved alleles.
func (g *Genetics) SetGenes(a []Alleles) { g.genes = append(g.genes[:0], a...) }

// Find returns a gene's alleles by name.
func (g *Genetics) Find(name string) (Alleles, bool) {
	for i, gene := range g.cfg.Genes {
		if gene.Name == name && i < len(g.genes) {
			return g.genes[i], true
		}
	}
	return Alleles{}, false
}

// FromParents gives a baby its genes from its parents (nil for a parent
// without genetics) and runs the birth events.
func (g *Genetics) FromParents(env *Env, a, b *Genetics) {
	r := env.World.Random()
	g.genes = g.genes[:0]
	for _, gene := range g.cfg.Genes {
		pa, pb := parentAlleles(a, gene, r), parentAlleles(b, gene, r)
		g.genes = append(g.genes, g.cross(gene, pa, pb, r))
	}
	g.FireEvents(env)
}

func parentAlleles(p *Genetics, gene Gene, r behavior.Random) Alleles {
	if p != nil {
		if al, ok := p.Find(gene.Name); ok && gene.Alleles.has(al.Main) && gene.Alleles.has(al.Hidden) {
			return al
		}
	}
	return Alleles{gene.Alleles.draw(r), gene.Alleles.draw(r)}
}

// cross picks one allele from each parent in a random order, then
// mutates each at the mutation rate.
func (g *Genetics) cross(gene Gene, a, b Alleles, r behavior.Random) Alleles {
	first, second := b, a
	if r.NextBool() {
		first, second = a, b
	}
	var out Alleles
	if !gene.Simplified && !r.NextBool() {
		out.Main = first.Hidden
	} else {
		out.Main = first.Main
	}
	if gene.Simplified || r.NextBool() {
		out.Hidden = second.Hidden
	} else {
		out.Hidden = second.Main
	}
	rate := gene.MutationRate
	if rate < 0 {
		rate = g.cfg.MutationRate
	}
	if r.NextFloat() <= rate {
		out.Main = gene.Alleles.draw(r)
	}
	if r.NextFloat() <= rate {
		out.Hidden = gene.Alleles.draw(r)
	}
	return out
}

// FireEvents runs the birth event of every variant the alleles fit.
func (g *Genetics) FireEvents(env *Env) {
	ctx := env.tester(env.Self, nil)
	for i, gene := range g.cfg.Genes {
		if i >= len(g.genes) {
			return
		}
		al := g.genes[i]
		for _, v := range gene.Variants {
			if fits(v, al) {
				env.fire(v.Birth, ctx)
			}
		}
	}
}

func fits(v GeneticVariant, al Alleles) bool {
	hit := false
	if v.Main.set() {
		if !v.Main.has(al.Main) {
			return false
		}
		hit = true
	}
	if v.Hidden.set() {
		if !v.Hidden.has(al.Hidden) {
			return false
		}
		hit = true
	}
	if v.Either.set() {
		if !v.Either.has(al.Main) && !v.Either.has(al.Hidden) {
			return false
		}
		hit = true
	}
	if v.Both.set() {
		if !v.Both.has(al.Main) || !v.Both.has(al.Hidden) {
			return false
		}
		hit = true
	}
	return hit
}

// AngerSound is one of anger_level's on_increase_sounds.
type AngerSound struct {
	Condition func(behavior.Mob) float32
	Sound     int
}

// AngerLevelConfig is minecraft:anger_level (1.21.3 schema 0x140560900;
// defaults were not read and are zero).
type AngerLevelConfig struct {
	AngryThreshold, AngryBoost, MaxAnger int32
	// DecrementInterval is in ticks.
	DecrementInterval                        int32
	DefaultAnnoyingness, ProjectileAnnoyance int32
	RemoveTargetsBelowThreshold              bool
	Nuisance                                 *definition.Filter
	Sounds                                   []AngerSound
}

type nuisance struct {
	id       behavior.ID
	anger    int32
	priority bool
}

// AngerLevel is the warden's anger (1.21.3 AngerLevelComponent::tick
// 0x14056FBA0, tryIncreaseAngerAt 0x140570D10). The lambda that adds the
// anger and _getListeningSoundEvent were not read: anger is capped at
// max_anger and the first sound whose condition holds plays.
type AngerLevel struct {
	cfg       AngerLevelConfig
	next      uint64
	top       int32
	nuisances []nuisance
}

// NewAngerLevel makes the component.
func NewAngerLevel(c AngerLevelConfig) *AngerLevel { return &AngerLevel{cfg: c} }

// Config returns the settings.
func (a *AngerLevel) Config() AngerLevelConfig { return a.cfg }

// SetConfig replaces the settings.
func (a *AngerLevel) SetConfig(c AngerLevelConfig) { a.cfg = c }

// Anger is the anger at the most annoying nuisance seen at the last
// decrement.
func (a *AngerLevel) Anger() int32 { return a.top }

// AngerAt is the anger at one entity.
func (a *AngerLevel) AngerAt(id behavior.ID) int32 {
	for _, n := range a.nuisances {
		if n.id == id {
			return n.anger
		}
	}
	return 0
}

// Top is the most annoying nuisance: angry ones first, then the
// prioritised (players), then by anger.
func (a *AngerLevel) Top() (behavior.ID, bool) {
	if len(a.nuisances) == 0 {
		return behavior.NoID, false
	}
	return a.nuisances[0].id, true
}

func (a *AngerLevel) canBeNuisance(env *Env, o behavior.Actor) bool {
	return o != nil && o.Alive() && o.ID() != env.Self.ID() && (a.cfg.Nuisance == nil || a.cfg.Nuisance.Eval(env.tester(env.Self, o)))
}

// Tick takes one anger off every nuisance each interval, forgets the
// ones at zero or no longer a nuisance, and resorts them.
func (a *AngerLevel) Tick(env *Env) {
	now := env.World.CurrentTick()
	step := uint64(max(a.cfg.DecrementInterval, 1))
	if a.next == 0 || a.next+step < now {
		a.next = now
	}
	for now >= a.next {
		top := int32(0)
		kept := a.nuisances[:0]
		for _, n := range a.nuisances {
			n.anger--
			if o := env.World.Actor(n.id); o != nil {
				if !a.canBeNuisance(env, o) {
					continue
				}
				top = max(top, n.anger)
			}
			if n.anger >= 0 {
				kept = append(kept, n)
			}
		}
		a.nuisances = kept
		a.sort()
		a.top = top
		a.next += step
	}
}

func (a *AngerLevel) sort() {
	t := a.cfg.AngryThreshold
	sort.SliceStable(a.nuisances, func(i, j int) bool {
		x, y := a.nuisances[i], a.nuisances[j]
		if (x.anger >= t) != (y.anger >= t) {
			return x.anger >= t
		}
		if x.priority != y.priority {
			return x.priority
		}
		return x.anger > y.anger
	})
}

// Increase adds anger at o when it can be a nuisance, plays the
// listening sound when asked, and makes a mob whose target is not a
// player drop it for a player that just turned it angry.
func (a *AngerLevel) Increase(env *Env, o behavior.Actor, amount int32, sound bool) {
	if !a.canBeNuisance(env, o) {
		return
	}
	i := -1
	for k := range a.nuisances {
		if a.nuisances[k].id == o.ID() {
			i = k
		}
	}
	if i < 0 {
		a.nuisances = append(a.nuisances, nuisance{id: o.ID(), priority: o.IsPlayer()})
		i = len(a.nuisances) - 1
	}
	n := &a.nuisances[i]
	n.anger = min(n.anger+amount, a.cfg.MaxAnger)
	anger := n.anger
	a.sort()
	m := env.Self
	if sound {
		for _, s := range a.cfg.Sounds {
			if s.Condition == nil || s.Condition(m) > 0 {
				m.PlaySound(s.Sound)
				break
			}
		}
	}
	if !o.IsPlayer() {
		return
	}
	if t := m.Target(); t != behavior.NoID {
		if cur := env.World.Actor(t); cur != nil && !cur.IsPlayer() && anger >= a.cfg.AngryThreshold {
			m.SetTarget(behavior.NoID)
		}
	}
}

// WeightedTrigger is one weighted choice of ravager_blocked's
// reaction_choices; a nil trigger is the choice to do nothing.
type WeightedTrigger struct {
	Weight  int32
	Trigger *definition.Trigger
}

// RavagerBlockedConfig is minecraft:ravager_blocked. Its 1.26.60 parser
// (sub_726B6C0) and the code that runs it were not read: Blocked picks a
// reaction by weight.
type RavagerBlockedConfig struct {
	KnockbackStrength float32
	Reactions         []WeightedTrigger
}

// Blocked runs a reaction picked by weight; the knockback is the
// engine's.
func (c *RavagerBlockedConfig) Blocked(env *Env) {
	total := int32(0)
	for _, r := range c.Reactions {
		total += max(r.Weight, 0)
	}
	if total <= 0 {
		return
	}
	pick := env.World.Random().NextInt(total)
	for _, r := range c.Reactions {
		if pick -= max(r.Weight, 0); pick < 0 {
			env.fire(r.Trigger, env.tester(env.Self, nil))
			return
		}
	}
}

// SpellEffect is one of spell_effects' add_effects.
type SpellEffect struct {
	Effect              string
	Duration, Amplifier int32
	Ambient, Visible    bool
	ScreenAnimation     bool
}

// SpellEffectsConfig is minecraft:spell_effects: the effects added and
// removed when the component is added. Its parser (1.26.60 sub_6FCABB0)
// was not decompiled and its initialize was not read; durations are taken
// as seconds and kept as ticks.
type SpellEffectsConfig struct {
	Add    []SpellEffect
	Remove []string
}

// SpellEffects applies its effects on the first tick after it is added.
type SpellEffects struct {
	cfg     SpellEffectsConfig
	applied bool
}

// NewSpellEffects makes the component.
func NewSpellEffects(c SpellEffectsConfig) *SpellEffects { return &SpellEffects{cfg: c} }

// Config returns the settings.
func (s *SpellEffects) Config() SpellEffectsConfig { return s.cfg }

// SetConfig replaces the settings; they apply again.
func (s *SpellEffects) SetConfig(c SpellEffectsConfig) { s.cfg, s.applied = c, false }

// Tick removes and adds the effects once.
func (s *SpellEffects) Tick(m Mob, act Actions) {
	if s.applied || act == nil {
		return
	}
	s.applied = true
	for _, name := range s.cfg.Remove {
		act.RemoveEffect(m, name)
	}
	for _, e := range s.cfg.Add {
		act.GiveEffect(m, e.Effect, e.Duration, e.Amplifier, e.Ambient)
	}
}
