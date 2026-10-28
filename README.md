<div align="center">

<img src=".github/helix-mark.svg" alt="Helix" width="120">

<p>
  <b>Vanilla Minecraft: Bedrock Edition mob AI</b><br>
  Goals, pathfinding, movement and spawning, standard library only
</p>

[![Minecraft](https://img.shields.io/badge/minecraft-v1.26.60_(Bedrock)-black)](https://feedback.minecraft.net/hc/en-us/sections/360001186971-Release-Changelogs)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Status](https://img.shields.io/badge/status-in_progress-orange)](#status)

</div>

## Status

Status as of 2026-10-28: **the AI core runs, and 170 of the 175 vanilla goals are built** (BDS lists 7 more names it no longer runs), ported from
BDS 1.26.52 and checked against 1.26.60.

- **Goals:** `goal` runs a mob's goals the way the BDS goal selector does (priorities, control
  flags, the target pass and the normal pass, NoAI).
- **Pathfinding:** `path` is the BDS pathfinder for walking and swimming mobs: A* search, node
  classification and costs.
- **Navigation:** `nav` has all seven navigation types (walk, float, generic, climb, swim, fly and
  hover): following a path, shortcuts, doors, sun avoidance, the stuck checks and fly gravity.
- **Movement:** `move` has the move controls (`minecraft:movement.*`); `physics` moves mobs like the
  BDS movement pipeline: travel types and speeds, swept box collision with depenetration and
  auto-step, gravity, drag, friction, climbing, jumping, and swimming and floating.
- **Behaviours:** `behavior` has 170 goals: strolling, swimming, flying and hovering, panic, tempt,
  following, looking, breeding, avoiding, every common target goal, melee, ranged and charge
  attacks, and the mob-specific ones (squid, slime, creeper, ravager, breeze, goat, fox, phantom,
  nautilus, cat, the warden, frog, strider, panda, axolotl, wolf, turtle, silverfish, guardian, vex, allay, cat gifts and the copper golem),
  the spell goals (summon_entity, send_event), and the
  villager and village goals (sleep, work, mingle, farming, hiding, trading, celebrating). The
  [roadmap](#roadmap) lists them all.
- **Spawning:** `spawn` is the BDS natural spawner: the per-chunk tick, floor search, population
  caps, spawn rules (biome filters, herds, permutes, delays, density) and each mob class's own
  spawn check.

What is left:

- **The other 8 goals**, entity definitions (loading a mob's components from Mojang's JSON) and
  the entity packets.
- **Physics:** water currents, bouncy blocks, vehicles and gliding.
- **Spawning:** structure spawn lists, phantoms, the zombie special case and the delay filter's
  first value. BDS's two random streams are one `Rand` here, and the Guardian sky check uses sky
  light.
- **Small gaps in built goals:** the panic start component, the float rider component,
  hurt_by_target's alert_same_type and breed's stop details. Block lists match by block name only,
  not by states or tags. Small differences from BDS are logged and fixed in one refinement pass at
  the end.
- **1.26.60 check:** a scan of every goal's code against 1.26.52 found panic changed (it keeps
  running while the hurt still counts and picks a new spot when its path ends) and vanilla moved
  ranged_attack to its `attack_range`/`attack_interval` objects; both are in. 1.26.60 also added a
  sound event and moved "undefined" from 614 to 615, which every goal's "no sound" now uses. The scan compares
  constants, strings and calls, not every line, so a small change can still be missed.
- **Not tested in game yet.** The goals are unit tested but not yet used by a running server or
  compared with BDS mob by mob.
- **The API is not stable yet.** Names and behaviour may still change between commits.

## How it is checked

Everything comes from the decompiled Bedrock Dedicated Server (BDS) 1.26.52, and goals added or
changed since come from 1.26.60: the same rules, the
same numbers and the same random draws in the same order, quirks included. Each goal cites the
BDS function addresses it was read from. The decompiled code is **not** in this repo; it was only
used to read how the game works. Clean-room research notes are in [`research/`](research/).

## Why entity?

Custom Bedrock servers usually write mob AI that only looks like vanilla: mobs wander, chase and
spawn differently, and farms and redstone contraptions that depend on vanilla behaviour break.
entity aims to be a one-to-one copy of how BDS runs mobs. It is part of
[Helix](https://github.com/helix-bedrock), an open-source Bedrock server engine in Go, but every
package talks to the world only through small interfaces (`World`, `Mob`, `Navigator`), so any Go
server can use it.

## Quick Start

```shell
go get github.com/helix-bedrock/entity
```

```go
// w implements behavior.World; m implements behavior.Targeter (a Mob that can
// target) for one of your mobs.
var goals goal.Selector
goals.Add(1, behavior.NewNearestAttackable(w, m, behavior.DefaultNearestAttackable))
goals.Add(6, behavior.NewStroll(w, m, behavior.DefaultStroll))
goals.Add(7, behavior.NewLookAt(w, m, behavior.DefaultLookAt))

// Every tick, from the mob's own tick:
goals.Tick(noAI)
```

A goal reaches the world only through the `World` and `Mob` it was made with, so each mob's goals
run on that mob's tick and mobs can be ticked on many threads.

| Package | What it does |
|---|---|
| [`goal`](goal) | The goal selector |
| [`behavior`](behavior) | The vanilla goals (`minecraft:behavior.*`) |
| [`path`](path) | The pathfinder |
| [`nav`](nav) | The navigation components |
| [`move`](move) | The move controls |
| [`physics`](physics) | Mob movement and collision |
| [`spawn`](spawn) | The natural spawner and spawn rules |

### What belongs where

entity is mob AI: what a mob decides to do and how it moves there. The things players, mobs and
blocks share belong to the engine, and entity reaches them only through interfaces:

- **Items and inventories** are the engine's: item stacks, stacking, damage, recipes
  (`behavior.ItemRules`) and containers (`behavior.Container`). entity keeps only the mob's own
  item decisions from its JSON components: which items it wants, shares, admires, barters for or
  equips.
- **Physics** is shared with players, but stays here until the engine API is designed. `physics`
  depends on no AI package (only the shared types in `path`), so it can move into the engine
  whole. Player movement itself is the engine's.

## Features

- **Same mobs as vanilla:** goals, paths, movement and spawning follow BDS's own code.
- **Data-driven:** goal settings are the JSON fields of Mojang's `minecraft:behavior.*`
  components, with BDS's defaults; spawn rules are read from Mojang's own files.
- **Replaceable:** every goal is a plain value; add, remove or write your own.
- **No dependencies:** only the Go standard library and helix-data's data package.

## Roadmap

| Part | State |
|---|---|
| Goal selector | ✅ done |
| Pathfinding (walk, swim) | ✅ done |
| Navigation (all seven types) | ✅ done |
| Move controls, physics | 🟡 done except water currents, bouncy blocks, vehicles, gliding |
| Wandering: random_stroll, random_swim, random_fly, random_breach, random_hover, float_wander, swim_idle, swim_wander, random_look_around (and _and_sit), random_sitting | ✅ written from BDS |
| Following: follow_owner, follow_parent, follow_mob, teleport_to_owner, tempt, mount_pathing | ✅ written from BDS |
| Looking: look_at_player, look_at_entity, look_at_target, look_at_trading_player | ✅ written from BDS |
| Survival: panic, float, swim_up_for_breath, flee_sun, restrict_sun, avoid_mob_type, avoid_block, run_around_like_crazy | 🟡 written from BDS; panic and float miss one component each |
| Moving to places: move_to_water, _lava, _liquid, _land, _block, _random_block, move_towards_target, move_towards_home/dwelling_restriction, stomp_turtle_egg | ✅ written from BDS |
| Breeding, sitting, timers: breed, stay_while_sitting, timer_flag_1 to 3 | 🟡 written from BDS; breed's stop details left |
| Targeting: nearest_attackable_target, nearest_prioritized_attackable_target, hurt_by_target, owner_hurt_by_target, owner_hurt_target, defend_trusted_target, defend_village_target, target_when_pushed | 🟡 written from BDS; hurt_by_target's alert_same_type left |
| Attacks: melee_attack, melee_box_attack, leap_at_target, ranged_attack, delayed_attack, stomp_attack, charge_attack, hold_ground, swell | ✅ written from BDS |
| Mob attacks: ocelotattack, fire_at_target, jump_around_target, move_around_target, aquatic_charge_attack, ram_attack, stalk_and_pounce_on_target, knockback_roar, swoop_attack, circle_around_anchor | ✅ written from BDS |
| Squid and slime goals (9) | ✅ written from BDS |
| Warden: emerge, dig, roar, sniff, investigate_suspicious_location, sonic_boom | ✅ written from BDS |
| Frog and strider: croak, rise_to_liquid_level | ✅ written from BDS |
| Panda, fox, axolotl, wolf, cat: lay_down, scared, sneeze, nap, find_cover, play_dead, beg, ocelot_sit_on_block | ✅ written from BDS |
| Items: pickup_items, equip_item, share_items, admire_item, barter, charge_held_item, drink_milk, drink_potion, eat_carried_item, snacking | 🟡 written from BDS, with the shareables, admire_item, barter and equip_item rules (`Items`); the inventory (`Container`), item data (stack sizes, damage, armor slots, recipes) and food use come from the engine |
| Roll, lay_egg, jump_to_block, silverfish_merge_with_stone, silverfish_wake_up_friends | ✅ written from BDS |
| Villagers and villages: move_to_poi, sleep, work, work_composter, mingle, hide, move_to_village, stroll_towards_village, move_through_village, explore_outskirts, move_indoors, move_outdoors, go_home, harvest_farm_block, fertilize_farm_block, raid_garden, inspect_bookshelf, celebrate, celebrate_survive, offer_flower, take_flower, trade_interest, trade_with_player | 🟡 written from BDS; POIs, villages, crops, items and trades come from the engine; trade_interest's item cycling and some defaults are simplified |
| Caravans, leaders, hovering: follow_caravan, follow_target_leader, hover, float_tempt | ✅ written from BDS 1.26.60 |
| Spells, guardian, vex, play: summon_entity, send_event, guardian_attack, vex_copy_owner_target, vex_random_move, play | ✅ written from BDS 1.26.60 |
| The allay: stay_near_noteblock, go_and_give_items_to_noteblock, go_and_give_items_to_owner | ✅ written from BDS 1.26.60 |
| drop_item_for, transport_items (moving items is the engine's, through hooks) | ✅ written from BDS 1.26.60 |
| use_kinetic_weapon, wither_random_attack_pos_goal, wither_target_highest_damage (the weapon, dash state and damage tally are the engine's, through hooks) | ✅ written from BDS 1.26.60 |
| The other 5 goals (the dragon) | ⬜ not started |
| Natural spawning | 🟡 written from BDS; structure lists, phantoms and a few special cases left |
| Entity definitions and packets | ⬜ not started |

**The goal is to match the latest Minecraft version exactly**, and to follow each Minecraft update.

## Contributing

- **Found a bug or a difference from vanilla?** Open an [issue](https://github.com/helix-bedrock/entity/issues)
- **Want to contribute code?** Read the [contributing guide](https://github.com/helix-bedrock/.github/blob/main/CONTRIBUTING.md)
  and the [AI policy](https://github.com/helix-bedrock/.github/blob/main/AI_POLICY.md)

## License

entity is licensed under [MIT](LICENSE). Mojang's vanilla data it reads lives in
[helix-data](https://github.com/helix-bedrock/helix-data).
