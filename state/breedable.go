package state

import (
	"github.com/helix-bedrock/entity/behavior"
	"github.com/helix-bedrock/entity/definition"
)

// BreedType is one mate the mob breeds with: the mate's entity type, the
// baby it makes ("" for none) and the event fired on breeding. In 1.26.60
// data the mate and event come from breedable's breeds_with and the baby
// from minecraft:offspring's offspring_pairs; older files list all three
// in breeds_with.
type BreedType struct {
	Mate, Baby string
	Event      *definition.Trigger
}

// EnvironmentRequirement is one of environment_requirements: count blocks
// matching the descriptors within radius. The engine answers it.
type EnvironmentRequirement struct {
	Blocks []string
	Count  int32
	Radius float32
}

// BreedableConfig is minecraft:breedable.
type BreedableConfig struct {
	RequireTame, InheritTamed, BlendAttributes, AllowSitting bool
	CausesPregnancy, RequireFullHealth                       bool
	// ExtraBabyChance is the chance of each extra baby, up to 16.
	ExtraBabyChance float32
	// Cooldown is breed_cooldown in seconds.
	Cooldown    float32
	BreedItems  []Item
	BreedsWith  []BreedType
	LoveFilters *definition.Filter
	Environment []EnvironmentRequirement
	// TransformToItem is what a used breed item turns into.
	TransformToItem string
	// LoveTicks is how long a fed mob stays in love.
	LoveTicks int32
	// HeartEvery is the love timer mask of the heart particle event
	// (every 16 ticks), LoveEvent the actor event broadcast.
	HeartEvery int32
	LoveEvent  int
	// HeartParticle and HeartCount are the particles spread over the mob
	// when it falls in love.
	HeartParticle, HeartCount int
	// RefuseSound plays when the environment requirements are not met.
	RefuseSound int
}

// DefaultBreedable is BDS's breedable (1.26.60 schema 0x6FCAED0:
// require_tame true, allow_sitting false, extra_baby_chance 0,
// breed_cooldown 60, causes_pregnancy and require_full_health false;
// inherit_tamed and blend_attributes true from the 1.21.3 constructor
// 0x140598850; the 600-tick love time from setInLove 0x1405BAF70 and the
// refuse sound 254 from getInteraction 0x1405B18A0, a 1.21.3 number).
func DefaultBreedable() BreedableConfig {
	return BreedableConfig{
		RequireTame: true, InheritTamed: true, BlendAttributes: true, Cooldown: 60,
		LoveTicks: 600, HeartEvery: 15, LoveEvent: EventLoveParticles, RefuseSound: 254,
		HeartParticle: ParticleHeart, HeartCount: 20,
	}
}

// Birth is one baby a mating makes. The engine creates it (BDS's
// ActorFactory::createBornActor), adds it to the world, blends its
// attributes when BlendAttributes is set and gives it its parents' genes,
// then calls Breedable.Born.
type Birth struct {
	Baby    string
	Partner behavior.ID
	// Tame is set when the parent is tame and inherit_tamed is on; Owner is
	// then the parent's owner. The baby is always made persistent.
	Tame            bool
	Owner           behavior.ID
	BlendAttributes bool
	// LoveCause is the player who fed the parents, NoID for none.
	LoveCause behavior.ID
	event     *definition.Trigger
}

// Breedable is minecraft:breedable (1.21.3 BreedableComponent:
// tickBreedableComponent 0x14015C0A0, canMate 0x1405AEB50, mate
// 0x1405B5DE0, _handleMate 0x1405A22B0, _handlePregnancy 0x1405A3190,
// _canBreed 0x1405A0990, getInteraction 0x1405B18A0, _useBreedItem
// 0x1405A4890, meetsSittingRequirements 0x1405B5F50).
type Breedable struct {
	cfg       BreedableConfig
	love      int32
	cooldown  int32
	loveCause behavior.ID
}

// NewBreedable makes the component.
func NewBreedable(c BreedableConfig) *Breedable {
	return &Breedable{cfg: c, loveCause: behavior.NoID}
}

// Config returns the settings.
func (b *Breedable) Config() BreedableConfig { return b.cfg }

// SetConfig changes the settings.
func (b *Breedable) SetConfig(c BreedableConfig) { b.cfg = c }

// Love is the love timer in ticks.
func (b *Breedable) Love() int32 { return b.love }

// Cooldown is the breeding cooldown in ticks.
func (b *Breedable) Cooldown() int32 { return b.cooldown }

// LoveCause is the player who put the mob in love, NoID for none.
func (b *Breedable) LoveCause() behavior.ID { return b.loveCause }

func (b *Breedable) cooldownTicks() int32 { return int32(b.cfg.Cooldown * ticksPerSecond) }

// Tick counts the love timer and the cooldown down. A baby is never in
// love; heart particles go out every 16 ticks of love.
func (b *Breedable) Tick(env *Env) {
	m := env.Self
	if m.Flag(behavior.FlagBaby) {
		b.ResetLove(m)
		return
	}
	inLove := b.love > 0
	if inLove {
		b.love--
		if b.love&b.cfg.HeartEvery == 0 {
			m.ActorEvent(b.cfg.LoveEvent)
		}
	}
	m.SetFlag(behavior.FlagInLove, inLove)
	if b.cooldown > 0 {
		b.love = 0
		m.SetFlag(behavior.FlagInLove, false)
		b.cooldown--
	}
}

// ResetLove ends love and starts the cooldown.
func (b *Breedable) ResetLove(m Mob) {
	b.cooldown = b.cooldownTicks()
	b.love = 0
	m.SetFlag(behavior.FlagInLove, false)
}

// SetInLove puts the mob in love, caused by a player (nil for none).
func (b *Breedable) SetInLove(m Mob, player behavior.Actor) {
	b.love = b.cfg.LoveTicks
	b.loveCause = behavior.NoID
	if player != nil && !player.Removed() {
		b.loveCause = player.ID()
	}
	m.Particles(b.cfg.HeartParticle, b.cfg.HeartCount)
	m.SetFlag(behavior.FlagInLove, true)
}

func (b *Breedable) breedType(mate string) *BreedType {
	for i := range b.cfg.BreedsWith {
		if b.cfg.BreedsWith[i].Mate == mate {
			return &b.cfg.BreedsWith[i]
		}
	}
	return nil
}

// sittingOK is meetsSittingRequirements: a sitting mob breeds only with
// allow_sitting.
func (b *Breedable) sittingOK(a behavior.Actor) bool {
	return !a.Flag(behavior.FlagSitting) || b.cfg.AllowSitting
}

// CanMate reports that the mob can breed with partner, whose breedable
// component is pb (nil when it has none): neither cooling down, a breed
// type for the partner, tame rules, sitting rules and both in love.
func (b *Breedable) CanMate(self, partner behavior.Actor, pb *Breedable) bool {
	if partner == nil || partner.ID() == self.ID() || b.cooldown > 0 || pb == nil || pb.cooldown > 0 {
		return false
	}
	if b.breedType(partner.Type()) == nil {
		return false
	}
	st, pt := self.Flag(behavior.FlagTamed), partner.Flag(behavior.FlagTamed)
	if b.cfg.RequireTame {
		if !st || !pt {
			return false
		}
	} else if st != pt {
		return false
	}
	return b.sittingOK(self) && b.sittingOK(partner) &&
		self.Flag(behavior.FlagInLove) && partner.Flag(behavior.FlagInLove)
}

// Mate breeds the mob with partner (pb its breedable, nil for none). It
// returns the babies to make: one, then another while a random float is
// under extra_baby_chance, up to 16. With causes_pregnancy the mob becomes
// pregnant instead and no baby is returned. Love particles and experience
// orbs are the engine's.
func (b *Breedable) Mate(env *Env, partner behavior.Actor, pb *Breedable, dst []Birth) []Birth {
	m := env.Self
	if !m.Flag(behavior.FlagInLove) || partner == nil {
		return dst
	}
	if b.cfg.CausesPregnancy {
		b.pregnancy(env, partner, pb)
		return dst
	}
	r := env.World.Random()
	for i := 0; i < 16; i++ {
		if birth, ok := b.mateOnce(env, partner, pb); ok {
			dst = append(dst, birth)
		}
		if r.NextFloat() >= b.cfg.ExtraBabyChance {
			break
		}
	}
	return dst
}

// endLove resets both parents and returns the love cause: the mob's
// player, else the partner's.
func (b *Breedable) endLove(env *Env, partner behavior.Actor, pb *Breedable) behavior.ID {
	b.ResetLove(env.Self)
	if pb != nil {
		pb.love = 0
		pb.cooldown = pb.cooldownTicks()
		if pm, ok := partner.(behavior.Mob); ok {
			pm.SetFlag(behavior.FlagInLove, false)
		}
	}
	cause := b.loveCause
	if p := env.World.Actor(cause); p == nil || !p.IsPlayer() {
		cause = behavior.NoID
		if pb != nil {
			cause = pb.loveCause
		}
	}
	return cause
}

func (b *Breedable) mateOnce(env *Env, partner behavior.Actor, pb *Breedable) (Birth, bool) {
	cause := b.endLove(env, partner, pb)
	t := b.breedType(partner.Type())
	if t == nil || t.Baby == "" {
		return Birth{}, false
	}
	m := env.Self
	birth := Birth{
		Baby: t.Baby, Partner: partner.ID(), Owner: behavior.NoID,
		BlendAttributes: b.cfg.BlendAttributes, LoveCause: cause, event: t.Event,
	}
	if b.cfg.InheritTamed && m.Flag(behavior.FlagTamed) {
		birth.Tame = true
		birth.Owner = m.Owner()
	}
	return birth, true
}

// Born fires the breed type's event on the mob once the engine has made
// the baby, with the baby as the other subject.
func (b *Breedable) Born(env *Env, birth Birth, baby behavior.Actor) {
	env.fire(birth.event, env.tester(env.Self, baby))
}

func (b *Breedable) pregnancy(env *Env, partner behavior.Actor, pb *Breedable) {
	b.endLove(env, partner, pb)
	t := b.breedType(partner.Type())
	if t == nil {
		return
	}
	env.Self.SetFlag(FlagPregnant, true)
	env.fire(t.Event, env.tester(env.Self, partner))
}

// Feed is a player feeding the item in hand. It returns the breed item
// used, or -1 when the mob does not take it: cooling down, pregnant,
// already in love, untamed with require_tame, hurt with
// require_full_health, love_filters failing, no matching item, or the
// environment requirements unmet (then the refuse sound plays). envMet is
// the engine's answer to the environment requirements. The engine uses up
// the item and hands back transform_to_item.
func (b *Breedable) Feed(env *Env, player behavior.Actor, h Hand, envMet bool) int {
	m := env.Self
	if b.cooldown > 0 || m.Flag(FlagPregnant) || m.Flag(behavior.FlagInLove) ||
		b.cfg.RequireTame && !m.Flag(behavior.FlagTamed) {
		return -1
	}
	if b.cfg.RequireFullHealth {
		if hp, maxHP := m.Health(); hp < maxHP {
			return -1
		}
	}
	if !b.cfg.LoveFilters.Eval(env.tester(m, player)) {
		return -1
	}
	i := match(h, b.cfg.BreedItems)
	if i < 0 {
		return -1
	}
	if len(b.cfg.Environment) > 0 && !envMet {
		m.PlaySoundAt(b.cfg.RefuseSound, m.AttachPos(AttachHead))
		return -1
	}
	if !m.Flag(behavior.FlagInLove) {
		b.SetInLove(m, player)
	}
	return i
}
