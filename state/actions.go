package state

import "github.com/helix-bedrock/entity/behavior"

// Actions are the engine's hands for the components that act on the world
// or on other entities: hurting, effects, blocks, teleports, explosions
// and spawns. entity decides when and to whom; the engine does it.
//
// Several of these change other entities (Hurt, GiveEffect), so with more
// than one core the engine must order them the way it orders any other
// cross-entity change.
type Actions interface {
	// Hurt damages target by an actor damage cause, as Actor::hurt with an
	// ActorDamageByActorSource from by (behavior.NoID for no source).
	Hurt(target behavior.Actor, by behavior.ID, cause int, amount float32)
	// CanReceiveEffect is Actor::canReceiveMobEffectsFromGameplay and not
	// being invulnerable to an entity attack from by.
	CanReceiveEffect(target behavior.Actor, by behavior.ID) bool
	// GiveEffect adds a mob effect by name for a number of ticks.
	GiveEffect(target behavior.Actor, effect string, ticks, amplifier int32, ambient bool)
	// EffectTicks is the ticks left of an active effect, 0 for none.
	EffectTicks(target behavior.Actor, effect string) int32
	// RemoveEffect removes a mob effect by name.
	RemoveEffect(target behavior.Actor, effect string)
	// Teleport tries to move the mob to p as TeleportComponent::teleport
	// does (finding ground below p, checking the space and liquids, the
	// sound and particles); false when it cannot.
	Teleport(p behavior.Vec3) bool
	// PlaceBlock places the block a descriptor names at p when the block
	// there is air and the block may be placed (BlockLegacy::mayPlace),
	// with block update flags 3.
	PlaceBlock(p behavior.Pos, block string) bool
	// Explode makes an explosion caused by the mob.
	Explode(at behavior.Vec3, e Explosion)
	// Spawn spawns what a spawn_entity entry names from the mob.
	Spawn(e SpawnEntry)
	// TNTExplodes is the tntexplodes game rule.
	TNTExplodes() bool
}
