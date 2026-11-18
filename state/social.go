package state

import (
	"slices"

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

// TrustingInteraction matches BDS (1.26.60):
// a mob that does not trust yet offers feeding when the
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

// TryTrust is the feeding, as BDS 1.26.60's trusting callback: with the
// probability the mob becomes trusting, otherwise it broadcasts the failed-taming event
// (6). The eating event and using up the item come first and are the engine's.
func TryTrust(env *Env, c *TrustingConfig) bool {
	if env.World.Random().NextFloat() >= c.Probability {
		env.Self.ActorEvent(EventTameFail)
		return false
	}
	BecomeTrusting(env, c)
	return true
}

// BecomeTrusting matches BDS (0x1406240D0): the
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

// Nuisance is one entity the mob is angry at, as BDS 1.26.60 saves it
// under "Nuisances" (save, load).
type Nuisance struct {
	ID    behavior.ID
	Anger int32
	// Priority is BDS's "Priority" byte: set for entities that are not
	// players, which sort after players.
	Priority bool
}

// AngerLevel is the warden's anger, AngerLevelComponent of BDS 1.26.60:
// its suspects are a std::multiset kept in BDS's order (insert):
// angry ones first, then players, then by anger, later inserts after
// equal ones.
type AngerLevel struct {
	cfg  AngerLevelConfig
	next uint64
	// top is the component's: the highest anger at the last decrement,
	// raised by any increase above it.
	top       int32
	nuisances []Nuisance
}

// NewAngerLevel makes the component.
func NewAngerLevel(c AngerLevelConfig) *AngerLevel { return &AngerLevel{cfg: c} }

// Config returns the settings.
func (a *AngerLevel) Config() AngerLevelConfig { return a.cfg }

// SetConfig replaces the settings.
func (a *AngerLevel) SetConfig(c AngerLevelConfig) { a.cfg = c }

// Anger is the component's highest anger.
func (a *AngerLevel) Anger() int32 { return a.top }

// AngerFor is: the anger at the mob's target when it is a
// suspect, else the highest anger.
func (a *AngerLevel) AngerFor(target behavior.ID) int32 {
	if i := a.find(target); target != behavior.NoID && i >= 0 {
		return a.nuisances[i].Anger
	}
	return a.top
}

// AngerAt is the anger at one entity.
func (a *AngerLevel) AngerAt(id behavior.ID) int32 {
	if i := a.find(id); i >= 0 {
		return a.nuisances[i].Anger
	}
	return 0
}

// Top is the first suspect in the set's order.
func (a *AngerLevel) Top() (behavior.ID, bool) {
	if len(a.nuisances) == 0 {
		return behavior.NoID, false
	}
	return a.nuisances[0].ID, true
}

// Suspects appends the suspects' IDs in the set's order.
func (a *AngerLevel) Suspects(dst []behavior.ID) []behavior.ID {
	for _, n := range a.nuisances {
		dst = append(dst, n.ID)
	}
	return dst
}

// Calm is: the entity is not a suspect or its anger is below
// angry_threshold.
func (a *AngerLevel) Calm(id behavior.ID) bool {
	i := a.find(id)
	return i < 0 || a.nuisances[i].Anger < a.cfg.AngryThreshold
}

// Nuisances returns the suspects in the set's order, for saving.
func (a *AngerLevel) Nuisances() []Nuisance { return a.nuisances }

// Load adds saved suspects as BDS's load does, each inserted into the set.
func (a *AngerLevel) Load(ns []Nuisance) {
	for _, n := range ns {
		a.insert(n)
	}
}

func (a *AngerLevel) find(id behavior.ID) int {
	for i := range a.nuisances {
		if a.nuisances[i].ID == id {
			return i
		}
	}
	return -1
}

// before is the set's comparator: k sorts before n.
func (a *AngerLevel) before(k, n Nuisance) bool {
	t := a.cfg.AngryThreshold
	if k.Anger >= t {
		return n.Anger < t || !k.Priority && n.Priority || k.Priority == n.Priority && k.Anger > n.Anger
	}
	return n.Anger < t && (!k.Priority && n.Priority || k.Priority == n.Priority && k.Anger > n.Anger)
}

// insert puts n after every suspect it does not sort before (the
// multiset's insert).
func (a *AngerLevel) insert(n Nuisance) {
	i := 0
	for i < len(a.nuisances) && !a.before(n, a.nuisances[i]) {
		i++
	}
	a.nuisances = slices.Insert(a.nuisances, i, n)
}

// Valid is the suspect test: alive, the nuisance filter, and a
// player only when not invulnerable and in survival or adventure.
func (a *AngerLevel) Valid(env *Env, o behavior.Actor) bool {
	if o == nil || !o.Alive() {
		return false
	}
	if a.cfg.Nuisance != nil && !a.cfg.Nuisance.Eval(env.tester(env.Self, o)) {
		return false
	}
	if !o.IsPlayer() {
		return true
	}
	if o.Invulnerable() {
		return false
	}
	mode := o.GameMode()
	if mode == behavior.GameModeDefault {
		mode = env.World.DefaultGameMode()
	}
	return mode == -1 || mode == behavior.GameModeSurvival || mode == behavior.GameModeAdventure
}

// Tick matches BDS (per entity): the
// decrements due since the last one, then remove_targets_below_angry_threshold.
// The first tick stands for the component's init, which sets the next
// decrement one interval on.
func (a *AngerLevel) Tick(env *Env) {
	now := env.World.CurrentTick()
	step := uint64(max(a.cfg.DecrementInterval, 1))
	if a.next == 0 {
		a.next = now + step
	}
	for now >= a.next {
		a.decrement(env)
		a.next += step
	}
	if !a.cfg.RemoveTargetsBelowThreshold {
		return
	}
	m := env.Self
	if t := m.Target(); t != behavior.NoID && env.World.Actor(t) != nil && a.Calm(t) {
		m.SetTarget(behavior.NoID)
	}
}

// decrement is: one anger off every suspect; suspects that
// exist but are no longer valid go, missing ones stay until their anger
// runs out; the highest anger of the valid ones is kept.
func (a *AngerLevel) decrement(env *Env) {
	old := a.nuisances
	a.nuisances = make([]Nuisance, 0, len(old))
	top := int32(0)
	for _, n := range old {
		if o := env.World.Actor(n.ID); o != nil {
			if !a.Valid(env, o) {
				continue
			}
			top = max(top, n.Anger-1)
		}
		if n.Anger > 0 {
			n.Anger--
			a.insert(n)
		}
	}
	a.top = top
}

// Increase is: anger at a valid o goes up by amount, held
// between 0 and max_anger (a suspect at 0 is dropped), playing the first
// on_increase_sound whose condition holds at the head when asked; a mob
// whose target is not a player drops it for a player now angry enough.
func (a *AngerLevel) Increase(env *Env, o behavior.Actor, amount int32, sound bool) {
	if !a.Valid(env, o) {
		return
	}
	n := Nuisance{ID: o.ID(), Priority: !o.IsPlayer()}
	old := int32(0)
	if i := a.find(o.ID()); i >= 0 {
		n, old = a.nuisances[i], a.nuisances[i].Anger
	}
	v := max(old+amount, 0)
	if old+amount > a.cfg.MaxAnger {
		v = a.cfg.MaxAnger
	}
	a.top = max(a.top, v)
	if v != old {
		if i := a.find(o.ID()); i >= 0 {
			a.nuisances = slices.Delete(a.nuisances, i, i+1)
		}
		if v > 0 {
			n.Anger = v
			a.insert(n)
		}
	}
	m := env.Self
	if sound {
		a.playSound(m)
	}
	if !o.IsPlayer() {
		return
	}
	if t := env.World.Actor(m.Target()); t != nil && !t.IsPlayer() && !a.Calm(o.ID()) {
		m.SetTarget(behavior.NoID)
	}
}

// playSound plays the first sound whose condition is above 0.
func (a *AngerLevel) playSound(m Mob) {
	for _, s := range a.cfg.Sounds {
		if s.Condition == nil || s.Condition(m) > 0 {
			m.PlaySoundAt(s.Sound, m.AttachPos(AttachHead))
			return
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
// and the code that runs it were not read: Blocked picks a
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
// removed when the component is added. Its parser
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
