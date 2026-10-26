# valheim-save

Reads and edits Valheim character files (`.fch`): skills, inventory, the
map, spawn points and the used-cheats flag. Every edit first copies the file
beside itself as `<name>_backup_edit-<stamp>.fch`, named the way the game
names its own backups so they list together, and never overwrites one.

```bash
make build
bin/valheim-save locate
bin/valheim-save inspect hrafn
bin/valheim-save skills hrafn set blood_magic 100
bin/valheim-save items hrafn add "Staff of the Wild" 1 4
```

A character is a name, looked up where the game and Steam keep them, or the
path to a `.fch` file. Quit the game first: it writes the file on logout and
every twenty minutes, over whatever is there.

| | |
|---|---|
| `locate` | every character file on this machine |
| `inspect <character>` | what the file holds: health, knowledge, skills, the inventory with its slot grid |
| `backup <character>` | copy the file beside itself |
| `skills <character> [list \| set <skill\|all> <level>]` | the skills, 0 to 100 |
| `items <character> [list \| add <item> [count] [quality] \| remove <x>,<y>]` | the inventory |
| `map <character> [list \| reveal <world> \| wipe <world>]` | the fog of war and pins per world |
| `spawn <character> [list \| set <world> <x> <y> <z> \| clear <world>]` | the custom spawn point per world |
| `cheat <character> [show \| clear \| set]` | the profile's used-cheats flag |

## Why a file editor

Everything the server knows about a player is in the player's ZDO and what
the client tells it. Skills are neither: they are computed and stored on the
client, in the character file, and the only wire traffic about them is the
skill factor applied to an attack before it is sent. So nothing done on or
through a server can raise a skill. The console's `raiseskill` can, but it
is a cheat command, and running one sets a flag in the same file that the
achievements code reads for ever after. Editing the file sets the skill
without touching the flag.

The same holds for the inventory, the map (fog of war and pins), the
per-world spawn, logout and death points, and the flag itself: all client
state, all in the file.

The packages here were written in
[valheim-npc](https://github.com/pietjan/valheim-npc), whose NPCs speak the
same wire format, and copied out so that this can stand on its own: a
character file needs no server. The item table is regenerated from that
project's dev server (`make items`).

## Where the files are

The game saves under `AppData\LocalLow\IronGate\Valheim` on Windows and
`~/.config/unity3d/IronGate/Valheim` on Linux, in `characters` (synced) or
`characters_local`. A character that is on Steam Cloud is, on the player's
own machine, in Steam's local copy of the cloud store,
`Steam\userdata\<account>\892970\remote\characters`, and the game's own
`characters` folder is then empty but for `steam_autocloud.vdf`. Under WSL
both are reached through `/mnt/c`. `locate` walks all of these;
`internal/character.Roots` is the list. A Steam installed somewhere else is
not found; give the path.

Steam Cloud syncs the file and nothing more: an edited file is uploaded as
the new version the next time the game starts, with Steam running. If Steam
ever shows a sync conflict, choose the local copy.

## The layout

`PlayerProfile.SavePlayerToDisk` writes an `int32` length, a ZPackage, an
`int32` length and the SHA-512 of that package. The game reads the hash and
discards it, so a wrong hash loads, but the editor writes a right one.

The package is the profile: version (47 since 1.0.20, 46 before), the
statistics (ten difficulty tables of 205 counters each, plus named tallies),
the first-spawn flag, one record per world keyed by the world's UID (custom
spawn, logout and death points, home point, and the minimap as a byte
array), then the name, player ID, start seed, the used-cheats flag, the
creation date, and the player block as a byte array.

The player block is `Player.Save`: version (34 since 1.0.20, 33 before),
health, stamina, guardian power, the inventory, everything the character
knows (recipes, stations, materials, tutorials, uniques, trophies, biomes,
texts), appearance, the food slots, the skills, custom data, eitr, the build
menu's state, and since 34 two int-to-int tables of drop counters and item
upgrade seeds.

The skills block is a version (2), a count, and `(int32 type, float level,
float accumulator)` per skill the character has ever raised. A skill never
used is simply absent, and the editor keeps it that way on `set all`, as the
game's own `raiseskill all` does.

The minimap is a version (8) and a gzip-compressed package: the texture
size (2048), one byte per cell of the player's own fog and one per cell of
fog shared by others, the pins, and whether the player's position is public.
The game refuses a map of any other size.

`internal/character` decodes all of this and encodes it back in the same
order. `TestRoundTripRealFiles` reads every character file on the machine
and requires the encode to reproduce it byte for byte, map included (the map
is kept as the game's bytes until it is changed, since Go's gzip is not
.NET's byte for byte). Anything the decoder does not recognise is a refusal,
not a guess: profile versions other than 46 and 47, player data other than
33 and 34, item layouts other than 109 and 110. A field read at the wrong
width is silent garbage from there to the end of the file, and the skills
are near the end.

## Items

The inventory names an item by its prefab's hash and stores what the player
has: stack, quality, durability, crafter, the cheated bit. The limits are
the prefab's, so `internal/character/items.tsv` carries them for every item
the server knows: type, stack size, quality cap, durability and durability
per level, whether it teleports, its weight, and the English the game shows.
`make items` regenerates it through `tools/item-dump.cs`, a probe run inside
valheim-npc's dev server, and the game's text table. An item the table does
not know is shown by its hash.

`items add` makes the item the game would have crafted: full durability for
its quality, the character as its crafter when it is equipment (name and
player ID, which the tooltip shows on a weapon or armour piece and nobody
wants on a stack of wood), the cheated bit off. It tops up stacks of the
same prefab and quality first, then takes free slots in reading order, as
the game's own `AddItem` does, and says how many did not fit. The stack is
capped at the prefab's maximum because the game caps it on load. The quality
is not, because the game does not either: a quality past the cap loads,
shows its stars and its glow, and only refuses a further upgrade at a
bench. The inventory is eight wide and as many rows as were bought from the
trader, kept in the file as the unique key `invrows <n>`.

## What trips the cheat flag

`Achievements.IsCheatedAtAll` is four things OR'd together: the profile's
`m_usedCheats`, a cheated item in the inventory, world modifiers outside the
preset list, and a modded client. Only the console sets `m_usedCheats`, on
any command marked `isCheat`, and nothing clears it but this editor. An item
is cheated when its own bit is set, which the `spawn` command does, or when
its total damage exceeds 10000; the item only counts while it is in the
inventory. `cheat show` shows both the flag and any flagged item.

## World names

The profile keys worlds by UID and never writes the name beside it; the
names live in the world files (`.fwl`, or `_main.<n>.fwl2` inside a folder
named after the world). The editor reads those from the same roots and pairs
what it can. A world that lives only on a server has no local file, so
`map list` shows its number; `-worlds <dir>` adds a folder, such as a
server's `worlds_local`, and the number becomes a name.
