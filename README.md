# entity

The AI and pathfinding engine for Helix entities: entity definitions loaded
from Mojang's JSON files, components, behaviours (AI goals), sensors and
targeting, navigation, pathfinding, movement and spawn rules. It is a one-to-one
copy of how the Bedrock Dedicated Server (BDS) runs mobs.

**Status as of 2026-10-25:** `goal` runs a mob's goals the way
the BDS goal selector does; `path` is the BDS 1.26.52 pathfinder for walking
and swimming mobs (A* search, node classification and costs); `nav` has all
seven navigation types (walk, float, generic, climb, swim, fly and hover:
following a path, shortcuts, doors, sun avoidance, the stuck checks, fly
gravity); `move` has the move controls (minecraft:movement.*); `physics`
moves mobs like the BDS movement pipeline (travel types and speeds, swept
box collision with depenetration and auto-step, gravity, drag, friction,
climbing, jumping, and the swim and float navigation's travel). Water
currents, bouncy blocks, players, vehicles and gliding are not in
`physics` yet. `behavior` has the common goals (random_stroll, panic,
tempt, follow_owner, follow_parent, look_at_player and look_at_entity,
random_look_around, float, breed, avoid_mob_type, hurt_by_target,
nearest_attackable_target and melee_attack). These parts are not done yet:
the panic start component, the float rider component, hurt_by_target's
alert_same_type and breed's stop details. `spawn` is the BDS 1.26.52
natural spawner: the per-chunk tick, floor search, population caps, spawn
rules (biome filters, herds, permutes, delays, density) and each mob
class's own spawn check, behind a `World` interface the host implements.
Not in `spawn` yet: structure spawn lists, phantoms, the zombie special
case, and the delay filter's first value; BDS's two random streams are one
`Rand`, and the Guardian sky check uses sky light. Entity definitions are
not built yet. Research notes go in [`research/`](research/).

Part of [Helix](https://github.com/helix-bedrock), an open-source Minecraft: Bedrock
Edition server engine in Go.

## License

MIT. See [LICENSE](LICENSE).
