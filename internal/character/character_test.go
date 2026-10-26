package character

import (
	"bytes"
	"os"
	"testing"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// Every character file on this machine must survive a decode and encode with
// no change to any byte: that is the proof the layout is the game's and not
// an approximation of it. Skipped where there are none, which is CI.
func TestRoundTripRealFiles(t *testing.T) {
	found := Locate()
	if len(found) == 0 {
		t.Skip("no character files on this machine")
	}
	tried := 0
	for _, f := range found {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		p, err := Decode(data)
		if err != nil {
			// Old backups predate the layouts this package reads; the live
			// files must not.
			if f.IsBackup() {
				t.Logf("skip %s: %v", f.Path, err)
				continue
			}
			t.Errorf("%s: %v", f.Path, err)
			continue
		}
		tried++
		out, err := p.Encode()
		if err != nil {
			t.Errorf("%s: encode: %v", f.Path, err)
			continue
		}
		if !bytes.Equal(out, data) {
			t.Errorf("%s: round trip differs at byte %d of %d", f.Path, firstDiff(out, data), len(data))
		}
		if !Verify(data) {
			t.Logf("%s: hash does not match (the game ignores it)", f.Path)
		}
		// The map must survive its own round trip too, which is a content
		// comparison since gzip output is not the game's byte for byte.
		for _, w := range p.Worlds {
			m, err := w.Map()
			if err != nil {
				t.Errorf("%s: world %d: %v", f.Path, w.Key, err)
				continue
			}
			if m == nil {
				continue
			}
			var copyW WorldData
			if err := copyW.SetMap(m); err != nil {
				t.Errorf("%s: world %d: %v", f.Path, w.Key, err)
				continue
			}
			again, err := copyW.Map()
			if err != nil {
				t.Errorf("%s: world %d: re-decode: %v", f.Path, w.Key, err)
				continue
			}
			if again.Size != m.Size || !bytes.Equal(again.Explored, m.Explored) || !bytes.Equal(again.ExploredOthers, m.ExploredOthers) ||
				len(again.Pins) != len(m.Pins) || again.PublicPosition != m.PublicPosition {
				t.Errorf("%s: world %d: map round trip differs", f.Path, w.Key)
			}
		}
	}
	if tried == 0 {
		t.Skip("no readable character files on this machine")
	}
}

func firstDiff(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// A profile built here, with every optional field exercised, must come back
// equal, and its hash must verify.
func TestRoundTripSynthetic(t *testing.T) {
	p := sample()
	data, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(data) {
		t.Fatal("hash does not verify")
	}
	q, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := q.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, data) {
		t.Fatalf("round trip differs at byte %d", firstDiff(again, data))
	}
	if q.Name != "Tester" || q.Player.Skills.Get(SkillBloodMagic).Level != 44.5 || q.Player.Inventory.Items[1].Quality != 100 {
		t.Fatalf("decoded wrong: %+v", q)
	}
	m, err := q.Worlds[0].Map()
	if err != nil {
		t.Fatal(err)
	}
	if m.Size != 4 || m.ExploredCount() != 3 || len(m.Pins) != 1 || m.Pins[0].Author != "Steam_1" {
		t.Fatalf("map decoded wrong: %+v", m)
	}
}

func TestUnknownVersionRefused(t *testing.T) {
	p := sample()
	p.Version = 45
	if _, err := p.Encode(); err == nil {
		t.Fatal("encoded profile 45")
	}
	p.Version = ProfileCheatCleanSlate
	p.Player.Version = 35
	if _, err := p.Encode(); err == nil {
		t.Fatal("encoded player data 35")
	}
}

func TestSkillsSet(t *testing.T) {
	var s Skills
	s.Version = SkillsVersion
	s.Set(SkillRun, 10)
	s.List[0].Accumulator = 5
	s.Set(SkillRun, 100)
	if got := s.Get(SkillRun); got.Level != 100 || got.Accumulator != 0 {
		t.Fatalf("got %+v", got)
	}
	s.Set(SkillSwim, 1)
	if len(s.List) != 2 {
		t.Fatalf("got %d skills", len(s.List))
	}
	if tp, ok := ParseSkillType("blood_magic"); !ok || tp != SkillBloodMagic {
		t.Fatalf("parse blood_magic: %v %v", tp, ok)
	}
	if _, ok := ParseSkillType("dancing"); ok {
		t.Fatal("parsed dancing")
	}
}

func sample() *Profile {
	var w WorldData
	w.Key = 12345
	w.HaveCustomSpawn = true
	w.SpawnPoint = zpackage.Vector3{X: 1, Y: 2, Z: 3}
	w.HomePoint = zpackage.Vector3{X: 0, Y: 30, Z: 0}
	m := &Map{Version: MapPinsAuthor, Size: 4, Explored: make([]byte, 16), ExploredOthers: make([]byte, 16)}
	m.Explored[0], m.Explored[5], m.Explored[15] = 1, 1, 1
	m.Pins = []Pin{{Name: "home", Pos: zpackage.Vector3{X: 1}, Type: 5, Checked: true, OwnerID: 7, Author: "Steam_1"}}
	m.PublicPosition = true
	if err := w.SetMap(m); err != nil {
		panic(err)
	}
	stats := make([]StatTable, 2)
	for i := range stats {
		stats[i].Counters = make([]float32, 3)
		stats[i].EnemyStats = make([][]NamedFloat, 5)
	}
	stats[0].KnownWorlds = []NamedFloat{{"npcdev", 100}}
	stats[1].EnemyStats[2] = []NamedFloat{{"Greyling", 3}}
	return &Profile{
		Version:     ProfileCheatCleanSlate,
		Stats:       stats,
		FirstSpawn:  true,
		Worlds:      []WorldData{w},
		Name:        "Tester",
		PlayerID:    619588599,
		StartSeed:   "abc",
		DateCreated: 1700000000,
		Player: &Player{
			Version:       PlayerPostLaunch,
			MaxHealth:     100,
			Health:        50,
			MaxStamina:    75,
			GuardianPower: "GP_Eikthyr",
			Inventory: Inventory{Version: ItemNewCheats, Items: []Item{
				{DurabilityCenti: 10000, GridX: 0, GridY: 0, Quality: 1, Stack: 20, HasPrefab: true, PrefabHash: 123},
				{DurabilityCenti: 515000, GridX: 3, GridY: 2, PickedUp: true, Equipped: true, Quality: 100, Stack: 1, CrafterID: 1, CrafterName: "Hrafn", HasPrefab: true, PrefabHash: -5,
					CustomData: []NamedString{{"k", "v"}}, Cheated: true},
			}},
			KnownRecipes:  []string{"Recipe_Hammer"},
			KnownStations: []NamedInt{{"$piece_workbench", 2}},
			KnownTexts:    []NamedString{{"$text", "words"}},
			Beard:         "Beard1",
			Hair:          "Hair1",
			Model:         1,
			Foods:         []Food{{"$item_pancake", 100}},
			Skills:        Skills{Version: SkillsVersion, List: []Skill{{SkillBloodMagic, 44.5, 1}, {SkillRun, 51.1, 21.6}}},
			BuildUI:       []byte{1, 2, 3},
			DropCounters:  []IntPair{{5, 6}},
			UpgradeSeeds:  []IntPair{{7, 8}},
		},
	}
}
