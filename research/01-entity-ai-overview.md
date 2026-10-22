# Entity AI: overview (BDS 1.26.52)

Clean-room notes written in our own words. They contain no decompiled code,
addresses or symbol dumps.

**Build:** every claim comes from **BDS 1.26.52**. Most of it was read in the
Linux build's IDA database, with names from its RTTI and vtables; the rest
comes from its strings and the vanilla behaviour pack that ships with it. The
older 1.21.3 build was used only to put names on functions.

**Status as of 2026-10-22:** first pass. Every claim has a confidence level:
- high: read directly in 1.26.52;
- medium: read in 1.26.52, with part of the meaning inferred;
- low: inferred.

Nothing here has been checked in game yet; the [test list](#bds-test-list)
covers that.

## 1. Where an entity's instructions live

An entity has no hand-written "brain". Its behaviour is the sum of the
**components** that its behaviour-pack JSON turns on. The engine supplies
three things:

1. **Component implementations.** These hold the data and state.
2. **Systems.** These tick every entity that has a given component, for
   example the goal selector, navigation and move control systems.
3. **Goals.** These are the `minecraft:behavior.*` components, the AI tasks.

So:
- **What** a mob does is data: `entities/*.json` in a behaviour pack. Vanilla
  mobs use Mojang's vanilla pack, and add-on mobs use the same format.
- **How** each piece works is engine code, one implementation per component
  or goal name.

If Helix implements every name, any vanilla or add-on entity file loads
unchanged. (high)

**Counts in 1.26.52** (high: from the build's strings and classes):

| Kind | Count or names |
|---|---|
| `minecraft:behavior.*` names | **185** (about 187 goal classes) |
| Navigation | `navigation.walk`, `climb`, `float`, `fly`, `hover`, `swim` and `generic` |
| Movement | `movement.basic`, `amphibious`, `dolphin`, `fly`, `generic`, `glide`, `hover`, `jump`, `skip`, `sway` and `vex` |
| Jump | `jump.static` and `jump.dynamic` |
| Annotations | `annotation.break_door` and `annotation.open_door` |
| Event response kinds | about 25 (see 2.4) |
| Filter tests | about 95 (see 2.5) |
| Behaviour trees | a small separate system with selector and sequence nodes and a packet that sends a tree to the client; probably for the education agent (low) |

## 2. Loading entity JSON (add-on compatibility)

### 2.1 File shape
The top level is `format_version` plus `minecraft:entity`, which contains:
- `description`: identifier, spawnable and summonable flags, properties and
  more;
- `component_groups`;
- `components`;
- `events`.

The source is the vanilla behaviour pack that ships with BDS 1.26.52, which is
public data. (high)

### 2.2 Identifiers
Loading is rejected for:
- an identifier that starts with a number;
- a `minecraft:` identifier that does not override an existing entity.

(high: the error messages are in 1.26.52)

### 2.3 Components and component groups
- **Components.** Each key under `components` is looked up by name in a
  factory table:
  - an unknown name gives a "not parsed" or "no longer exists" error;
  - a value that is not an object is an error.

  (high)
- **Component groups.** Named bundles that events add or remove at run time.
  The live set of components is a stack: the base components plus every added
  group, in order. When it changes, the combined description is rebuilt and
  later groups override earlier ones. (medium)
- **Versioned definitions.** Many component and goal definitions exist in
  several **versioned copies**, one per `format_version` range. The version
  tags in 1.26.52 include 1.21.120, 1.21.130, 1.26.0, 1.26.10, 1.26.20, 1.26.30
  and 1.26.40. The parser picks the copy that matches the file's
  `format_version`, so old add-ons keep their old behaviour. Helix must do the
  same. (high that the copies exist; the exact ranges are open)

### 2.4 Events
Each event is an object. Its keys are:

| Key | Meaning |
|---|---|
| `add` / `remove` | component groups to add or remove |
| `filters` | the event only runs if they pass |
| `randomize` | a list of choices, each with a `weight` |
| `sequence` | a list of steps, each with its own filters |
| `trigger` | fire another event |

If both `randomize` and `sequence` are present, `sequence` is ignored with a
warning. (high)

**Response kinds in 1.26.52** (high: class names):
- command and queue_command;
- play_sound;
- damage and die;
- drop_item and spawn_loot;
- emit_particle, particle_effect and emit_vibration;
- add and remove mob effect;
- reset_target and set_home_position;
- set_property: it must target the entity itself and needs at least one
  change;
- shoot, swing and teleport;
- stop_movement and unleash;
- transform_item and decrement_stack;
- execute_event_on_home_block.

The JSON spelling of each one still needs checking against the vanilla pack.

### 2.5 Filters
A filter tree uses `all_of`, `any_of` and `none_of`, with tests at the leaves.
A test has these fields:
- `test`;
- `subject`: self, other, player, target, parent or damager;
- `operator`;
- `domain`;
- `value`.

There are about 95 test classes in 1.26.52. (high) Examples:
- properties: bool, enum, float and int properties, and has_property;
- equipment: has_equipment, has_equipment_tag and has_damaged_equipment;
- state: is_baby, is_sitting, is_sleeping, is_sneaking, is_panicking and
  is_navigating;
- places: in_water, in_lava, in_nether, in_village and in_clouds;
- distances: home_distance, owner_distance, target_distance and
  distance_to_nearest_player;
- world: moon_phase, light_level, temperature_type, dimension weather and
  biome_has_tag;
- newer tests: is_bound_to_creaking_heart and has_trade_supply.

An old "legacy" filter format is still accepted. TODO: list every JSON test
name.

## 3. The core engine: components and tick order

- **Storage.** Entities are EnTT entity IDs, and each component has its own
  storage. Systems iterate over the entities that have the components they
  need. (high)
- **Goal selector skips.** The goal selector system visits each entity that
  has its components (four storages, the smallest one is iterated) and skips
  one that:
  - has a "removed" flag set;
  - is dead (health 0 or less);
  - fails one virtual check, probably "is AI disabled" or "immobile".

  (high for the checks; medium for what they mean)
- **System order.** The full order is open. The likely order is sensors and
  targeting, then goals, then navigation, then move control, then travel and
  physics. (low)

## 4. Goals: how a mob picks what to do

Each JSON goal becomes a prioritised goal with:
- a **priority**: lower runs first;
- **control flags**: a bit mask on the goal, such as move, look and jump;
- a "used" (running) flag and a "to start" flag.

**The goal hooks** (high):
- can use;
- can continue to use;
- can be interrupted;
- start;
- stop;
- tick;
- is target goal.

### Per tick
The goal selector runs **two passes**: target goals first (`hurt_by_target`,
`nearest_attackable_target` and similar), then normal goals. Each pass tries
to start goals, then ticks them. (high)

**If the entity has a certain actor flag set, probably "no AI":** every goal
of both kinds is stopped and nothing new starts. (high for the behaviour;
medium for the flag's name)

### Try to start
For each goal of this pass's kind:

1. **Running goals.** If the goal is running and is now blocked, or can no
   longer continue, it gets its stop call and is marked not running.
2. **Starting.** A goal that is not running starts when both hold:
   - no other running goal blocks it;
   - its "can use" check passes.

   It is then marked running and "to start".
3. **Clean-up.** At the end, any running goal that the new starts now block
   is stopped.

### The blocking rule
Goal A is blocked by another **running** goal B in two cases:
- **B's priority is the same or better** (equal or lower number): A is blocked
  if both are the same kind (target or normal) and their control flags
  overlap. **Ties block:** at equal priority, the goal that is already running
  keeps running.
- **B's priority is worse** (higher number): A is blocked only if **B cannot
  be interrupted**. This is checked without looking at the flags, so a
  non-interruptible low-priority goal blocks everything above it, even goals
  with other flags. This is easy to get wrong.

(high)

### Tick
For each goal of this pass's kind: if it is marked "to start", it gets its
start call and the mark is cleared. Then, if it is running, it gets its tick
call. If the goal list was replaced during the tick (an event changed the
groups), every old goal is stopped. (high)

## 5. Navigation and pathfinding

**Navigation components** set:
- flags such as `can_open_doors`, `can_pass_doors`, `avoid_water`,
  `can_float`, `can_path_over_lava`, `avoid_damage_blocks` and more;
- one follower class: walk, climb (wall climber), float, fly, hover, swim
  (water bound) or generic.

(high)

### 5.1 How a goal moves a mob
1. The goal calls "move to" with a position or an entity and a speed.
2. Navigation creates a path with A*.
3. Each tick, navigation follows the path and gives a wanted position to the
   move control.
4. The move control turns that into a rotation, a forward speed and jumps.

### 5.2 A* search (high: confirmed in 1.26.52)
- **Open set:** a binary heap ordered by estimated total cost.
- **Heuristic:** the straight-line distance to the goal × **1.5**. This makes
  the search greedy rather than always finding the shortest path.
- **Expansion cap:** the loop stops after **140** expanded nodes.
- **Max range:** a node is skipped when its squared distance to the start is
  more than the square of the mob's path range, which is passed in.
- **Neighbours:** a fixed table of **18** offsets. The first pass checks
  which offsets are open and builds a bit mask. Diagonal and vertical moves
  are only tried when the straight moves next to them are open.
- **Neighbour buffer:** at most **32** candidate neighbours per node.
- **Edge cost:** the distance between the two nodes, plus the parent's cost,
  plus the neighbour node's cost (its malus), plus an extra per-node term.
  A cheaper route updates the node's parent and cost; otherwise the node is
  pushed onto the heap with the heuristic above.
- **Unreachable goal:** when the heap empties or the cap is hit, the result is
  a **partial path** to the node that got closest to the goal. If no node got
  closer than the start, there is no path.
- **Node costs:** blocked node types are impassable. `minecraft:preferred_path`
  gives per-block costs, using the block under the node. Other nodes use a
  default cost. (medium in 1.26.52; the full node-type list is open)

### 5.3 Following a path each tick (high for the shape)
1. The navigation tick counter goes up.
2. If the mob has a path that is not finished and the follower says it can
   update now (it is on the ground, or swimming, or flying, and so on), it
   runs the path update below.
3. **Path update.**
   - **Walkers** skip ahead past nodes at the same block height as the mob.
     Mobs with the "can fly" flag instead skip runs of nodes of the same node
     type.
   - Every node within **max(half the mob's width, 0.6)** blocks (compared
     as squared distances) counts as reached and is skipped.
   - The last node uses its own "arrival distance" setting instead.
   - Door nodes are aimed at the middle of the block.
   - **New in this build:** when a path node is a door-like block and the mob
     can open doors (an actor flag or a door component), path following
     handles it specially and can stop at that node. (medium)
4. **Wanted position.** The target point is the current path node's centre:
   - **Lowered:** if the mob is not flying and there is no air in a square
     the mob's width around it, the point is lowered by 1.
   - **Raised:**
     - to the top of the collision shape of a partial block there (slabs,
       carpets);
     - to the top of the collision shape of the block below;
     - by +1 when the mob is lower than the node and the block can be
       climbed by jumping (ladders and scaffolding, depending on the mob's
       climb components).
5. **Hand-off.** The point and the navigation speed go to the move control's
   "set wanted position".

### 5.4 Open for navigation
- The stuck check and repath timers.
- The exact condition for "can update path" in each follower class.
- The node-type list and each type's cost.

## 6. What a Helix in-game API needs to expose

This follows from the above. It is for the API design, which is not decided
yet.

- **Definitions.**
  - Load vanilla and add-on entity JSON, including the versioned behaviour
    for each `format_version`.
  - Register new components, goals, event responses and filter tests by name.
  - Add or remove component groups, fire events, and read and set entity
    properties.
- **Goals.**
  - The hook interface: can use, can continue, can be interrupted, start,
    stop, tick, is target goal, priority and control flags.
  - Add, remove or replace goals on one entity at run time.
  - Plugins can write goals in Go.
- **Navigation.**
  - Move to a position or an entity, and stop.
  - Read the current path.
  - Change the navigation flags.
  - Custom node costs, including a custom evaluator.
  - The search limits (cap and range) exposed, with vanilla as the default.
- **Movement.** Move, look and jump control: set a wanted position for
  scripted movement.
- **Filters.** Evaluate a filter tree against an entity and add new tests.
- **Sensors and targeting.** Read and set the target, and add sensors.

## Open questions

- The full system tick order and its position in the level tick.
- The exact actor flags behind "no AI", "can fly" and "can open doors"
  (numbers 16, 21 and 81 in the actor flag list).
- The JSON names of every event response and filter test.
- The `format_version` range for each versioned definition.
- The stuck and repath timers, and the node-type cost table.
- The move control maths: turn speed and jump.

## BDS test list

These are checks for a person in game.

- **Goal priority.** Hit a zombie while it chases a villager. It should switch
  target to you.
- **Ties.** Two goals with the same priority and the same flags: the one
  already running keeps going until it stops by itself.
- **Non-interruptible goal.** A mob in a non-interruptible low-priority goal
  (for example, a fox sleeping) should not start a higher-priority goal until
  that goal ends.
- **No AI.** `/summon` a zombie with `minecraft:is_immobile`, or with AI off,
  then turn AI back on. Its goals should start fresh.
- **Events and groups.** A baby animal grows up: its groups are added and
  removed. Test randomize by summoning 100 sheep and counting the wool colours
  against the vanilla weights.
- **Old add-on.** An add-on entity with an old `format_version` should behave
  the same as in BDS.
- **Path following.**
  - Zombie over slabs and carpets: it should not hop.
  - Spider climbing a wall.
  - Bee flying around a tree.
  - Villager walking through a door.
- **Partial path.** Put a player behind glass. The zombie should walk to the
  closest point and stay there.
- **Long maze.** The mob should give up or take a partial path once the route
  needs more than 140 nodes.
