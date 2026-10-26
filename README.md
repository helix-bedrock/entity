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
`physics` yet. `behavior` has 43 of the 170 goals vanilla entities use:
random_stroll, random_swim, random_fly, random_breach, random_hover, panic,
tempt, follow_owner, follow_parent, teleport_to_owner, look_at_player and
look_at_entity, random_look_around and random_look_around_and_sit, float,
breed, avoid_mob_type, hurt_by_target, owner_hurt_by_target,
owner_hurt_target, nearest_attackable_target, melee_attack,
melee_box_attack, leap_at_target, mount_pathing, run_around_like_crazy,
stay_while_sitting, flee_sun, restrict_sun, swim_idle, swim_wander,
swim_up_for_breath, float_wander, timer_flag_1 to 3,
move_towards_home_restriction, move_towards_dwelling_restriction,
move_to_water, move_to_lava, move_to_liquid, move_to_land and
stomp_turtle_egg. The other vanilla goals are not built yet. These parts
are not done yet:
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
