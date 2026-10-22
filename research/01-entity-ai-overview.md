# Entity AI: first overview

Clean-room notes written in our own words. They contain no decompiled code,
addresses or symbol dumps.

**Builds read**
- BDS 1.21.3 (Windows, with Mojang's PDB): used for names and structure.
- BDS 1.26.52 (Linux class and vtable names, and Windows): used to confirm
  that the classes still exist.

**Status as of 2026-10-22:** first pass. Every claim has a confidence level:
- high: read directly in one build and consistent with the other;
- medium: read in 1.21.3 only;
- low: inferred.

Nothing here has been checked in game yet; the [test list](#bds-test-list)
covers that.

## 1. Where an entity's instructions live

An entity has no hand-written "brain". Its behaviour is the sum of the
**components** that its behaviour-pack JSON switches on. The engine supplies
three things:

1. **Component implementations.** These hold the data and state,
   for example health, movement speed, navigation and breathable.
2. **Systems.** These tick every entity that has a given component, for
   example the goal selector system, the navigation system and the move
   control system.
3. **Goals.** These are the `minecraft:behavior.*` components, the AI tasks
   such as wandering, attacking and following.

So "where the instructions are written" has two parts:
- **What** a mob does is data: the JSON in `entities/*.json` of a behaviour
  pack. Vanilla mobs come from Mojang's vanilla behaviour pack, and add-on
  mobs use the same format.
- **How** each piece works is engine code: one implementation per component
  or goal name. Helix must therefore re-implement every component and goal
  name. It can then load any vanilla or add-on entity file unchanged.
  (high)

Scale in 1.26.52: about 187 goal classes and 13 move or body control kinds:
- move controls: generic, fly, glide, hop, hover, swim, dolphin,
  amphibious and slime;
- body controls: legacy and agent;
- jump controls: dynamic;
- look controls: agent.

There is also a small behaviour-tree system, with selector and sequence
nodes and a packet that sends a tree to the client. (high: class names in
1.26.52)

## 2. Loading entity JSON (add-on compatibility)

### File shape
The top level is `format_version` plus `minecraft:entity`, which contains:
- `description`: identifier, spawnable and summonable flags, properties and
  more;
- `component_groups`;
- `components`;
- `events`.

The source is Mojang's bedrock-samples behaviour pack, which is public. (high)

### Identifiers
Loading is rejected for:
- an identifier that starts with a number;
- a `minecraft:` identifier that does not override an existing entity.

(high: the error messages are in both builds)

### Components
Each key under `components` is looked up in a factory table by name:
- a name that is not registered gives an error that the component is not
  parsed or no longer exists;
- a value that is not a JSON object is an error.

Some old names are special-cased, for example balloon, `custom_hit_test` and
two vex behaviours. (high)

### `component_groups`
Named bundles of components that events add or remove at run time. The
entity's live set of components is therefore a **stack** of definitions: the
base components plus every group that has been added, in order. When groups
change, the engine rebuilds the combined description, and later groups
override earlier ones. (medium: inferred from the add, remove, rebuild and
stack operations in the definition diff list)

### `events`
Each event is an object. Its keys are:

| Key | Meaning |
|---|---|
| `add` / `remove` | `component_groups` lists to add or remove |
| `filters` | the event only runs if they pass |
| `randomize` | a list of choices, each with a `weight` |
| `sequence` | a list of steps; each runs if its own filters pass |
| `trigger` | fire another event |

If both `randomize` and `sequence` are present, `sequence` is ignored with a
warning. (high: these are the loader's own strings) Later formats add more
response kinds, such as set_property, queue_command, emit_vibration and
reset_target. Those still need checking.

### Filters
A filter tree uses `all_of`, `any_of` and `none_of`, with tests at the leaves.
A test has these fields:
- `test`;
- `subject`: self, other, player, target, parent or damager;
- `operator`;
- `domain`;
- `value`.

The known test names in 1.21.3 include:
- is_family, has_component, is_biome and has_equipment;
- is_daytime, is_difficulty, is_game_rule and is_owner;
- is_target, is_underground, is_variant and is_altitude;
- is_brightness, in_caravan and is_snow_covered;
- many more in 1.26.52.

An old "legacy" filter format is still accepted and converted. (high)
TODO: the full list of test names for 1.26.52.

### Versioning
In 1.26.52 many component and goal definitions exist in **several versioned
copies**, one per `format_version` range. Examples are 1.21.130, 1.26.0 and
1.26.10–1.26.40. The parser picks the copy that matches the file's
`format_version`. Old add-ons therefore keep their old behaviour. Helix must
do the same, or old add-ons will break. (high: the versioned type names are in
1.26.52)

## 3. The core engine: components and tick order

- **Storage.** Entities are EnTT-style entity IDs, with each component in its
  own storage. Systems iterate over the entities that have the components they
  need. (high)
- **AI systems.** These run in this order (medium; the exact order of the
  whole system list is still open):
  1. sensors and targeting;
  2. the goal selector;
  3. navigation (path following);
  4. move control;
  5. the travel and physics step.
- The goal selector **skips** an entity that is dead (health 0 or less) or
  that fails one other check, probably "AI disabled". (high)

## 4. Goals: how a mob picks what to do

Each goal from the JSON becomes a prioritised goal: a priority from
`"priority"`, where lower runs first, plus control flags.

**Control flags** are a bit mask such as move, look and jump. Two goals that
share a flag cannot run together. (high)

Every tick, the goal selector runs **two passes**: first the target goals
(`nearest_attackable_target`, `hurt_by_target` and similar), then the normal
goals. Each pass has two steps:

1. **Try to start goals.** For each goal of the current kind:
   - If it is already running, check whether it must stop. It stops if it can
     no longer continue, or if another running goal blocks it. A goal is
     blocked by any running goal of **higher priority** that it shares a
     control flag with.
   - If it is not running, it starts when both of these hold:
     - No running goal blocks it. A lower-priority running goal that shares a
       flag and can be interrupted does not block it; that goal is pushed out
       instead.
     - Its own "can use" check passes.
   - After the loop, any running goal that the new goals now block is stopped.
2. **Tick goals.** Each goal marked "to start" gets its start call. Then every
   running goal gets its tick call.

(high for the structure; the exact rule for equal priorities still needs to
be confirmed)

A goal therefore has a fixed set of hooks:
- can use;
- can continue to use;
- can be interrupted;
- start;
- stop;
- tick;
- is target goal.

This is the interface Helix goals need.

## 5. Navigation and pathfinding

**Navigation types** come from the `minecraft:navigation.*` components:

| Navigation | Used by |
|---|---|
| walk | land mobs |
| climb | spiders (wall climber) |
| float | ghasts |
| fly | bees, parrots |
| hover | the vex and similar |
| swim | water mobs (water bound) |
| generic | mobs that can both walk and swim |

Each component sets flags, such as `can_open_doors`, `avoid_water`,
`can_float`, `can_path_over_lava` and `avoid_damage_blocks`, plus one
navigation class. (high)

### How a goal moves a mob
1. The goal asks navigation to "move to" a position or an entity at a speed.
2. Navigation **creates a path** using the A* search described in the
   private pathfinding note:
   - weighted heuristic of ×1.5;
   - an expansion cap of about 140 nodes;
   - neighbour tables of 8 or 18 offsets;
   - per-node costs, including `preferred_path`;
   - a partial path to the closest node if the goal cannot be reached.

   (medium, 1.21.3)
3. Each tick, navigation **follows** the path:
   - **Skip ahead.** Skip path nodes the mob has already reached. A walker
     skips to the first node at a different Y; a flier skips a run of nodes
     of the same type. A node counts as reached within
     max(half the mob's width, 0.6) blocks.
   - **Wanted position.** Work out the next point to aim for. Its height is
     adjusted to the top of partial blocks under it (slabs, carpets and
     similar). It is raised by 1 if the block can be climbed by jumping.
   - **Hand off.** Give that point and the path speed to the **move
     control**, which turns it into a rotation, a forward speed and a jump.

   (high for the shape; the constants are from 1.21.3)
4. **Stuck check.** Navigation keeps a "last stuck check" position and
   gives up or repaths when the mob has not moved. The timing is open.

## 6. What a Helix in-game API needs to expose

This follows from the above. It is for the API design, not decided yet.

- **Definitions.**
  - Load vanilla and add-on entity JSON.
  - Register new component names and goal names.
  - Look up an entity's definition stack, and add or remove component groups.
  - Fire entity events.
- **Goals.**
  - The goal hook interface: can use, can continue, can be interrupted,
    start, stop, tick and control flags.
  - Add, remove or replace goals on one entity at run time.
  - Let plugins write their own goals in Go.
- **Navigation.**
  - Move to a position or an entity, and stop.
  - Read the current path.
  - Change the navigation flags.
  - Plug in a custom node evaluator or cost function.
- **Move, look and jump control.** Set a wanted position directly, for
  scripted movement.
- **Filters.** Evaluate a filter tree against an entity and register new
  filter tests.
- **Sensors and targeting.** Read and set the target, and add sensors.

## Open questions

- The full system tick order and its position in the level tick.
- The exact rule for blocking and interrupting between goals of equal
  priority.
- The repath and stuck timers in navigation.
- All event response kinds and all filter test names in 1.26.52.
- How `format_version` picks each versioned definition: the exact ranges.
- Where the behaviour tree system is used (it may be education or agent only).
- What changed in pathfinding between 1.21.3 and 1.26.52. All the A* numbers
  above are from 1.21.3.

## BDS test list

These are checks for a person in game.

- **Goal priority.** Hit a zombie while it chases a villager. It should switch
  target, through the hurt_by_target priority.
- **Control flags.** A cow tempted by wheat should stop wandering at once and
  should not look around randomly while following.
- **Events and groups.** Feed a baby animal until it grows up: the `add` and
  `remove` of groups. Name a villager, then trade, then check whether its
  profession group stays.
- **Randomize.** Spawn 100 sheep with `/summon` and count the wool colours
  against the weights in the vanilla JSON.
- **Add-on.** Load a third-party add-on entity with an old `format_version`
  (1.16 or earlier) and check that it behaves the same as in BDS.
- **Path following.**
  - Zombie over slabs and carpets: it should not hop.
  - Zombie over fences: it should not jump over them.
  - Spider climbing a wall.
  - Bee flying around a tree.
- **Partial path.** Put a player behind glass. The zombie should walk to the
  closest point and stay there.
- **Long maze.** Does the mob give up beyond about 140 nodes? This confirms
  the cap.
