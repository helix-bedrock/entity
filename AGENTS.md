# AGENTS.md

Rules for AI coding agents working in this repo. Also read the org-wide rules in
`../.github/AGENT_RULES.md` (online:
<https://github.com/helix-bedrock/.github/blob/main/AGENT_RULES.md>). Where they
disagree, this file wins for this repo.

## What this repo is

entity is its own engine: the **behavior engine** for Minecraft: Bedrock Edition
entities. It runs an entity's components (the `minecraft:behavior.*` goals, the
movement, navigation and other components) and moves mobs with its physics.

It is not part of the main engine, [Helix](https://github.com/helix-bedrock/helix),
and must never be merged into it. Helix, or any other server, plugs into entity
through entity's interfaces. In repos and docs the main engine is called Helix.

entity does **not** own:

- **Spawning.** Natural spawning and spawn rules belong to the main engine, which
  knows the world, its regions and its tick. Components that make something
  spawn (`minecraft:spawn_entity`, breeding) decide that it happens and ask the
  engine to do it. The `spawn` package still lives here and is moving to Helix.
- **Items and inventories.** Item stacks, stacking, damage, recipes and
  containers are the engine's, reached through interfaces. entity keeps only the
  mob's own item decisions from its components.
- **Players.** Player movement and player items are the engine's.

## Commands

```bash
go build ./...
go vet ./...
golangci-lint run
go test -race ./...
go test -run=^$ -bench=. -benchmem ./...   # benchmarks
```

## Layout

| Package | Owns |
|---|---|
| `goal` | The goal selector |
| `behavior` | The components that are goals (`minecraft:behavior.*`) |
| `path` | The pathfinder |
| `nav` | The navigation components |
| `move` | The move controls (`minecraft:movement.*`) |
| `physics` | Mob movement and collision |
| `definition` | Entity files: component groups, events, filters, properties |
| `component` | Builds a mob's goals, navigation, movement, sensors and state from its components |
| `sensor` | Timers, the scheduler and the sensors that fire a mob's events |
| `state` | A mob's own state: age, breeding, taming, anger, healing, despawning, flags, interactions, riding, transforming, breathing, leashing, attributes, equipment, attack cooldown, physics-style components (buoyancy, pushing, rail sensor, body settings), timers, teleporting, trails, area attacks, genetics, anger, and the settings of the components the engine runs |
| `spawn` | The natural spawner (moving to Helix) |

## Invariants: do not break these

1. **Assume nothing, ask the engine.** entity never hard-codes or reads block
   IDs, block names, block states or item names, because every server
   represents them its own way. Every question about the world goes to the
   engine: is there a block here, is it air, liquid or solid, does it block
   motion, does it match this descriptor. No goal compares block or item names
   itself.
2. **Descriptors are passed through, never parsed.** A block or item list in a
   component's JSON stays as Mojang writes it (a name, states or tags). entity
   hands it to the engine to match or to place. BDS's default lists are kept as
   descriptors too, the same data Mojang ships.
3. **Code components, never entities.** An entity is only the list of components
   in its JSON, so once a component exists every entity that lists it works. No
   code checks an entity type, unless BDS's own component code does.
4. **Baseline plus your own.** The Minecraft components are the baseline. Anyone
   can add new components or override any baseline one, including a
   `minecraft:` name, and every entity that lists that name gets the override.
5. **Physics is a baseline too.** The BDS movement pipeline is the default,
   built from steps (input, travel type, speed, gravity, drag, friction,
   collision, step-up). A server can replace, wrap or drop any step, or the
   whole pipeline, per mob type or per mob.
6. **Everything can change, also while running.** BDS behaviour is only the
   default and is never locked in:
   - every gameplay number and sound, event or particle ID goes in the
     component's config, with the BDS value as its default; only pure maths
     stays as code constants;
   - defaults come from functions (`DefaultX()`), never package variables;
   - every component can be read and given new settings while it runs
     (`Config`, `SetConfig`), from the mob's own tick;
   - goals can be added, removed, replaced and reordered while the mob runs.
7. **BDS is the reference for the defaults.** Default behaviour matches the
   latest BDS (1.26.60; goals ported earlier came from 1.26.52) one to one,
   quirks included, and each port cites the BDS function addresses it was
   read from. Small differences are logged and fixed in one refinement pass
   at the end.

The code does not follow rules 1, 2, 4, 5 and 6 everywhere yet. New code must
follow them, and old code is being moved over.

## Gotchas

- Never paste decompiled code into this repo. It is used only to read how BDS
  works; the research notes are private.
- Python scripts on Windows write CRLF line endings unless they open files with
  `newline="\n"`. Run `gofmt -w` after scripted edits.
- Commits go straight to main, signed and signed off (`git commit -S -s`).
- Refresh the README's "Status as of <date>" line on every commit.
