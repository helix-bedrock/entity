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
gravity); `move` has the move controls (minecraft:movement.*). The swim and
float navigation's travel (physics), the goals themselves and entity
definitions are not built yet. Research notes go in [`research/`](research/).

Part of [Helix](https://github.com/helix-bedrock), an open-source Minecraft: Bedrock
Edition server engine in Go.

## License

MIT. See [LICENSE](LICENSE).
