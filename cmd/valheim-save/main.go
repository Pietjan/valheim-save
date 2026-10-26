// Command valheim-save reads and edits Valheim character files.
//
// Skills, the map, spawn points and the cheat flag live in the character
// file on the player's machine, not on the server, so none of them can be
// changed through the query host the way the world can. This edits the file
// instead: it decodes the whole thing, changes the one field, and writes it
// all back, with a backup beside it first. See internal/character for the
// format and the README for the why.
//
//	valheim-save locate
//	valheim-save inspect <character>
//	valheim-save backup <character>
//	valheim-save skills <character> [list | set <skill> <level> | set all <level>]
//	valheim-save map <character> [list | reveal <world> | wipe <world>]
//	valheim-save spawn <character> [list | set <world> <x> <y> <z> | clear <world>]
//	valheim-save cheat <character> [show | clear | set]
//	valheim-save items <character> [list | add <item> [count] [quality] | remove <x>,<y>]
//
// World names come from the world files on this machine; -worlds adds a
// folder, such as a server's worlds_local, for worlds that live elsewhere.
//
// A character is a name, looked up where the game and Steam keep them, or a
// path to a .fch file. Every edit writes <name>_backup_edit-<stamp>.fch
// beside the file first, and no backup is ever overwritten.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pietjan/valheim-save/internal/character"
	"github.com/pietjan/valheim-save/internal/zpackage"
)

func main() {
	flag.Var(&worldDirs, "worlds", "an extra folder of world files to take world names from (repeatable)")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	err := run(args[0], args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// worldDirs is -worlds: where to find world files beyond the game's own
// folders, for a world that lives on a server, such as docker/config/worlds_local.
var worldDirs stringList

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(s string) error { *l = append(*l, s); return nil }

func usage() {
	fmt.Fprint(os.Stderr, `usage: valheim-save [-worlds <dir>] <command> [<character>] [args]

  locate                                   list every character file on this machine
  inspect <character>                      print what the file holds
  backup <character>                       copy the file beside itself
  skills <character> [list]                print the skills
  skills <character> set <skill> <level>   set one skill, or "all"
  map <character> [list]                   per-world map: cells explored, pins
  map <character> reveal <world>           lift the fog over the whole world
  map <character> wipe <world>             put the fog back everywhere
  spawn <character> [list]                 per-world spawn, logout, death and home points
  spawn <character> set <world> <x> <y> <z>  set a custom spawn point
  spawn <character> clear <world>          go back to the bed or the start
  cheat <character> [show|clear|set]       the profile's used-cheats flag
  items <character> [list]                 the inventory, with the slot grid
  items <character> add <item> [count] [quality]  put an item in: a prefab or the game's name
  items <character> remove <x>,<y>         take the item out of a slot

A character is a name (looked up in Steam Cloud and the game's own folders)
or the path to a .fch file. A world is its name as "map list" prints it, or
its numeric key; names come from the world files on this machine, and
-worlds <dir> adds a folder of them, such as a server's worlds_local.
Every edit first copies the file to <name>_backup_edit-<stamp>.fch beside it.
`)
}

func run(cmd string, args []string) error {
	if cmd == "locate" {
		return locate()
	}
	if len(args) == 0 {
		return fmt.Errorf("%s needs a character", cmd)
	}
	path, err := resolve(args[0])
	if err != nil {
		return err
	}
	args = args[1:]
	switch cmd {
	case "inspect":
		return inspect(path)
	case "backup":
		out, err := character.Backup(path)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	case "skills":
		return skills(path, args)
	case "map":
		return mapCmd(path, args)
	case "spawn":
		return spawn(path, args)
	case "cheat":
		return cheat(path, args)
	case "items":
		return items(path, args)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

// resolve turns a name or a path into the file to edit. A name that exists
// in more than one store is an error: editing one copy while the game reads
// the other would look like the edit did nothing.
func resolve(arg string) (string, error) {
	if strings.HasSuffix(strings.ToLower(arg), character.Ext) || strings.ContainsAny(arg, `/\`) {
		if _, err := os.Stat(arg); err != nil {
			return "", err
		}
		return arg, nil
	}
	found := character.Find(arg)
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no character named %q; run locate", arg)
	case 1:
		return found[0].Path, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q is in more than one place, give the path:\n", arg)
	for _, f := range found {
		fmt.Fprintf(&b, "  %s\n", f.Path)
	}
	return "", errors.New(strings.TrimSpace(b.String()))
}

func locate() error {
	found := character.Locate()
	if len(found) == 0 {
		fmt.Println("no character files found under:")
		for _, r := range character.Roots() {
			fmt.Println(" ", r)
		}
		return nil
	}
	for _, f := range found {
		if f.IsBackup() {
			continue
		}
		fmt.Printf("%-6s %-12s %8d  %s\n", f.Source, f.Name, f.Size, f.Path)
	}
	return nil
}

// edit loads, hands the profile to fn, and saves it, with a backup of the
// file as it was first. The backup is taken after fn so that an edit that
// fails, such as a world that does not exist, leaves nothing behind.
func edit(path string, fn func(*character.Profile) error) error {
	p, err := character.Load(path)
	if err != nil {
		return err
	}
	if err := fn(p); err != nil {
		return err
	}
	out, err := character.Backup(path)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	fmt.Println("backup", out)
	if err := character.Save(path, p); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

func inspect(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := character.Decode(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Printf("%s  (profile v%d", p.Name, p.Version)
	if p.Player != nil {
		fmt.Printf(", player data v%d", p.Player.Version)
	}
	fmt.Printf(", %d bytes", len(data))
	if !character.Verify(data) {
		fmt.Print(", hash does not match")
	}
	fmt.Println(")")
	fmt.Printf("  player id   %d\n", p.PlayerID)
	fmt.Printf("  created     %s\n", time.Unix(p.DateCreated, 0).UTC().Format("2006-01-02"))
	fmt.Printf("  start seed  %s\n", p.StartSeed)
	fmt.Printf("  used cheats %v\n", p.UsedCheats)
	fmt.Printf("  worlds      %d\n", len(p.Worlds))
	if p.Player == nil {
		fmt.Println("  no player data: never spawned")
		return nil
	}
	pl := p.Player
	fmt.Printf("  health      %.0f / %.0f   stamina %.0f / %.0f   eitr %.0f / %.0f\n",
		pl.Health, pl.MaxHealth, pl.Stamina, pl.MaxStamina, pl.Eitr, pl.MaxEitr)
	if pl.GuardianPower != "" {
		fmt.Printf("  power       %s\n", pl.GuardianPower)
	}
	fmt.Printf("  knows       %d recipes, %d materials, %d stations, %d trophies, %d biomes\n",
		len(pl.KnownRecipes), len(pl.KnownMaterials), len(pl.KnownStations), len(pl.Trophies), len(pl.KnownBiomes))
	fmt.Printf("  skills      %d\n", len(pl.Skills.List))
	printSkills(pl)
	printInventory(pl)
	return nil
}

func printInventory(pl *character.Player) {
	rows := pl.InventoryRows()
	fmt.Printf("  inventory   %d of %d slots used, %d free (%d x %d)\n",
		len(pl.Inventory.Items), character.InventoryWidth*rows, pl.FreeSlots(), character.InventoryWidth, rows)
	grid := make([][]byte, rows)
	for y := range grid {
		grid[y] = []byte(strings.Repeat(". ", character.InventoryWidth))
	}
	for _, it := range pl.Inventory.Items {
		if int(it.GridY) < rows && int(it.GridX) < character.InventoryWidth {
			grid[it.GridY][2*int(it.GridX)] = '#'
		}
	}
	for _, row := range grid {
		fmt.Printf("    %s\n", strings.TrimRight(string(row), " "))
	}
	items := append([]character.Item(nil), pl.Inventory.Items...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].GridY != items[j].GridY {
			return items[i].GridY < items[j].GridY
		}
		return items[i].GridX < items[j].GridX
	})
	for _, it := range items {
		var tags []string
		if it.Equipped {
			tags = append(tags, "equipped")
		}
		if it.CrafterName != "" {
			tags = append(tags, "by "+it.CrafterName)
		}
		if it.Cheated {
			tags = append(tags, "CHEATED")
		}
		fmt.Printf("    %d,%d  %-28s x%-3d q%-4d dur %7.1f  %s\n", it.GridX, it.GridY, character.ItemLabel(it.PrefabHash), it.Stack, it.Quality, it.Durability(), strings.Join(tags, ", "))
	}
}

func items(path string, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		p, err := character.Load(path)
		if err != nil {
			return err
		}
		if p.Player == nil {
			return errors.New("no player data: never spawned")
		}
		printInventory(p.Player)
		return nil
	}
	switch {
	case args[0] == "add" && len(args) >= 2 && len(args) <= 4:
		count, quality := 1, 1
		var err error
		if len(args) >= 3 {
			if count, err = strconv.Atoi(args[2]); err != nil || count < 1 {
				return fmt.Errorf("count %q must be a whole number of at least 1", args[2])
			}
		}
		if len(args) == 4 {
			if quality, err = strconv.Atoi(args[3]); err != nil || quality < 1 || quality > 0xFFFF {
				return fmt.Errorf("quality %q must be a whole number from 1 to 65535", args[3])
			}
		}
		matches := character.FindItems(args[1])
		if len(matches) != 1 {
			if len(matches) == 0 {
				return fmt.Errorf("no item matches %q", args[1])
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%q matches %d items; give the prefab name:\n", args[1], len(matches))
			for i, m := range matches {
				if i == 20 {
					fmt.Fprintf(&b, "  ... and %d more\n", len(matches)-20)
					break
				}
				fmt.Fprintf(&b, "  %-28s %s\n", m.Prefab, m.Label())
			}
			return errors.New(strings.TrimSpace(b.String()))
		}
		info := matches[0]
		return edit(path, func(p *character.Profile) error {
			if p.Player == nil {
				return errors.New("no player data: never spawned")
			}
			left, err := p.AddItem(info, count, quality)
			if err != nil {
				return err
			}
			if left == count {
				return errors.New("inventory is full")
			}
			fmt.Printf("%s x%d q%d", info.Label(), count-left, quality)
			if info.IsEquipment() {
				fmt.Printf(" by %s", p.Name)
			}
			if left > 0 {
				fmt.Printf(" (%d did not fit)", left)
			}
			if quality > info.MaxQuality {
				fmt.Printf(" (the game's cap is %d; it loads, but a bench will not upgrade it)", info.MaxQuality)
			}
			fmt.Println()
			return nil
		})
	case args[0] == "remove" && len(args) == 2:
		var x, y int
		if _, err := fmt.Sscanf(args[1], "%d,%d", &x, &y); err != nil {
			return fmt.Errorf("slot %q must be <x>,<y> as the list prints it", args[1])
		}
		return edit(path, func(p *character.Profile) error {
			if p.Player == nil {
				return errors.New("no player data: never spawned")
			}
			it, ok := p.Player.RemoveItemAt(x, y)
			if !ok {
				return fmt.Errorf("slot %d,%d is empty", x, y)
			}
			fmt.Printf("removed %s x%d q%d from %d,%d\n", character.ItemLabel(it.PrefabHash), it.Stack, it.Quality, x, y)
			return nil
		})
	}
	return errors.New("usage: items <character> [list | add <item> [count] [quality] | remove <x>,<y>]")
}

func printSkills(pl *character.Player) {
	list := append([]character.Skill(nil), pl.Skills.List...)
	sort.Slice(list, func(i, j int) bool { return list[i].Level > list[j].Level })
	for _, s := range list {
		fmt.Printf("    %-16s %6.2f  (+%.1f)\n", s.Type, s.Level, s.Accumulator)
	}
}

func skills(path string, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		p, err := character.Load(path)
		if err != nil {
			return err
		}
		if p.Player == nil {
			return errors.New("no player data: never spawned")
		}
		printSkills(p.Player)
		return nil
	}
	if args[0] != "set" || len(args) != 3 {
		return errors.New("usage: skills <character> set <skill|all> <level>")
	}
	level, err := strconv.ParseFloat(args[2], 32)
	if err != nil || level < 0 || level > character.MaxSkillLevel {
		return fmt.Errorf("level must be a number from 0 to %d", character.MaxSkillLevel)
	}
	var types []character.SkillType
	if args[1] == "all" {
		types = character.SkillTypes()
	} else {
		t, ok := character.ParseSkillType(args[1])
		if !ok {
			var names []string
			for _, t := range character.SkillTypes() {
				names = append(names, t.String())
			}
			return fmt.Errorf("unknown skill %q; one of %s", args[1], strings.Join(names, ", "))
		}
		types = []character.SkillType{t}
	}
	return edit(path, func(p *character.Profile) error {
		if p.Player == nil {
			return errors.New("no player data: never spawned")
		}
		for _, t := range types {
			// "all" means every skill the character has touched, as the
			// game's own raiseskill all does; a skill never used stays off
			// the list rather than appearing from nowhere.
			if args[1] == "all" && p.Player.Skills.Get(t) == nil {
				continue
			}
			p.Player.Skills.Set(t, float32(level))
			fmt.Printf("%s %.2f\n", t, level)
		}
		return nil
	})
}

// worldByArg finds a world by its name in the known-worlds tally or by its
// key. The profile keys worlds by UID and names them only in the statistics,
// so a name is matched there and must map to exactly one key that has data.
func worldByArg(p *character.Profile, arg string) (*character.WorldData, error) {
	if key, err := strconv.ParseInt(arg, 10, 64); err == nil {
		for i := range p.Worlds {
			if p.Worlds[i].Key == key {
				return &p.Worlds[i], nil
			}
		}
		return nil, fmt.Errorf("no world with key %d", key)
	}
	names := worldNames(p)
	var hits []int64
	for key, name := range names {
		if strings.EqualFold(name, arg) {
			hits = append(hits, key)
		}
	}
	if len(hits) != 1 {
		return nil, fmt.Errorf("world %q matches %d worlds; use the key from \"map list\"", arg, len(hits))
	}
	for i := range p.Worlds {
		if p.Worlds[i].Key == hits[0] {
			return &p.Worlds[i], nil
		}
	}
	return nil, fmt.Errorf("world %q has no data", arg)
}

// worldNames maps world keys to names. The profile keys worlds by UID and
// names them only in the statistics tally, with no link between the two, so
// the names come from the world files on this machine, and a character that
// has played exactly one world gets that one paired from the tally. A world
// that lives only on a server somebody else runs stays a number.
func worldNames(p *character.Profile) map[int64]string {
	out := map[int64]string{}
	for key, w := range character.WorldNames(worldDirs...) {
		out[key] = w.Name
	}
	var names []string
	seen := map[string]bool{}
	for _, t := range p.Stats {
		for _, nf := range t.KnownWorlds {
			if !seen[nf.Name] {
				seen[nf.Name] = true
				names = append(names, nf.Name)
			}
		}
	}
	if len(names) == 1 && len(p.Worlds) == 1 {
		if _, ok := out[p.Worlds[0].Key]; !ok {
			out[p.Worlds[0].Key] = names[0]
		}
	}
	return out
}

func describeWorld(p *character.Profile, w *character.WorldData) string {
	if name, ok := worldNames(p)[w.Key]; ok {
		return fmt.Sprintf("%s (%d)", name, w.Key)
	}
	return strconv.FormatInt(w.Key, 10)
}

func mapCmd(path string, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		p, err := character.Load(path)
		if err != nil {
			return err
		}
		for i := range p.Worlds {
			w := &p.Worlds[i]
			m, err := w.Map()
			if err != nil {
				return err
			}
			if m == nil {
				fmt.Printf("%s: no map\n", describeWorld(p, w))
				continue
			}
			cells := int(m.Size) * int(m.Size)
			fmt.Printf("%s: %d x %d, %.1f%% explored, %d pins, position %s\n",
				describeWorld(p, w), m.Size, m.Size, 100*float64(m.ExploredCount())/float64(cells), len(m.Pins),
				map[bool]string{true: "public", false: "private"}[m.PublicPosition])
			for _, pin := range m.Pins {
				fmt.Printf("    %-12s %-24q at %7.0f %7.0f\n", pin.Type, pin.Name, pin.Pos.X, pin.Pos.Z)
			}
		}
		return nil
	}
	if len(args) != 2 || (args[0] != "reveal" && args[0] != "wipe") {
		return errors.New("usage: map <character> reveal|wipe <world>")
	}
	return edit(path, func(p *character.Profile) error {
		w, err := worldByArg(p, args[1])
		if err != nil {
			return err
		}
		m, err := w.Map()
		if err != nil {
			return err
		}
		if m == nil {
			return fmt.Errorf("%s has no map to %s", describeWorld(p, w), args[0])
		}
		before := m.ExploredCount()
		if args[0] == "reveal" {
			m.Reveal()
		} else {
			m.Wipe()
		}
		fmt.Printf("%s: explored %d -> %d cells\n", describeWorld(p, w), before, m.ExploredCount())
		return w.SetMap(m)
	})
}

func spawn(path string, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		p, err := character.Load(path)
		if err != nil {
			return err
		}
		for i := range p.Worlds {
			w := &p.Worlds[i]
			fmt.Printf("%s:\n", describeWorld(p, w))
			point := func(label string, have bool, v zpackage.Vector3) {
				if have {
					fmt.Printf("    %-7s %8.1f %7.1f %8.1f\n", label, v.X, v.Y, v.Z)
				} else {
					fmt.Printf("    %-7s none\n", label)
				}
			}
			point("spawn", w.HaveCustomSpawn, w.SpawnPoint)
			point("logout", w.HaveLogoutPoint, w.LogoutPoint)
			point("death", w.HaveDeathPoint, w.DeathPoint)
			point("home", true, w.HomePoint)
		}
		return nil
	}
	switch {
	case args[0] == "set" && len(args) == 5:
		var v zpackage.Vector3
		for i, f := range []*float32{&v.X, &v.Y, &v.Z} {
			x, err := strconv.ParseFloat(args[2+i], 32)
			if err != nil {
				return fmt.Errorf("coordinate %q: %w", args[2+i], err)
			}
			*f = float32(x)
		}
		return edit(path, func(p *character.Profile) error {
			w, err := worldByArg(p, args[1])
			if err != nil {
				return err
			}
			w.HaveCustomSpawn = true
			w.SpawnPoint = v
			fmt.Printf("%s: spawn %s\n", describeWorld(p, w), v)
			return nil
		})
	case args[0] == "clear" && len(args) == 2:
		return edit(path, func(p *character.Profile) error {
			w, err := worldByArg(p, args[1])
			if err != nil {
				return err
			}
			w.HaveCustomSpawn = false
			fmt.Printf("%s: spawn cleared\n", describeWorld(p, w))
			return nil
		})
	}
	return errors.New("usage: spawn <character> [list | set <world> <x> <y> <z> | clear <world>]")
}

func cheat(path string, args []string) error {
	if len(args) == 0 || args[0] == "show" {
		p, err := character.Load(path)
		if err != nil {
			return err
		}
		fmt.Printf("used cheats: %v\n", p.UsedCheats)
		if p.Player != nil {
			for _, it := range p.Player.Inventory.Items {
				if it.Cheated {
					fmt.Printf("cheated item in inventory: %s at %d,%d\n", character.ItemLabel(it.PrefabHash), it.GridX, it.GridY)
				}
			}
		}
		return nil
	}
	if len(args) != 1 || (args[0] != "clear" && args[0] != "set") {
		return errors.New("usage: cheat <character> [show|clear|set]")
	}
	return edit(path, func(p *character.Profile) error {
		p.UsedCheats = args[0] == "set"
		fmt.Printf("used cheats: %v\n", p.UsedCheats)
		return nil
	})
}
