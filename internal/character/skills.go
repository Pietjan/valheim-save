package character

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// SkillsVersion is the only skills layout the game has written since the
// accumulator was added. Skills.Save in the game.
const SkillsVersion = 2

// MaxSkillLevel is the ceiling the game clamps a skill to.
const MaxSkillLevel = 100

// SkillType is Skills.SkillType in the game.
type SkillType int32

// The skills, with the game's own numbering: the gaps are real.
const (
	SkillSwords         SkillType = 1
	SkillKnives         SkillType = 2
	SkillClubs          SkillType = 3
	SkillPolearms       SkillType = 4
	SkillSpears         SkillType = 5
	SkillBlocking       SkillType = 6
	SkillAxes           SkillType = 7
	SkillBows           SkillType = 8
	SkillElementalMagic SkillType = 9
	SkillBloodMagic     SkillType = 10
	SkillUnarmed        SkillType = 11
	SkillPickaxes       SkillType = 12
	SkillWoodCutting    SkillType = 13
	SkillCrossbows      SkillType = 14
	SkillJump           SkillType = 100
	SkillSneak          SkillType = 101
	SkillRun            SkillType = 102
	SkillSwim           SkillType = 103
	SkillFishing        SkillType = 104
	SkillCooking        SkillType = 105
	SkillFarming        SkillType = 106
	SkillCrafting       SkillType = 107
	SkillDodge          SkillType = 108
	SkillRide           SkillType = 110
)

var skillNames = map[SkillType]string{
	SkillSwords: "Swords", SkillKnives: "Knives", SkillClubs: "Clubs", SkillPolearms: "Polearms",
	SkillSpears: "Spears", SkillBlocking: "Blocking", SkillAxes: "Axes", SkillBows: "Bows",
	SkillElementalMagic: "ElementalMagic", SkillBloodMagic: "BloodMagic", SkillUnarmed: "Unarmed",
	SkillPickaxes: "Pickaxes", SkillWoodCutting: "WoodCutting", SkillCrossbows: "Crossbows",
	SkillJump: "Jump", SkillSneak: "Sneak", SkillRun: "Run", SkillSwim: "Swim", SkillFishing: "Fishing",
	SkillCooking: "Cooking", SkillFarming: "Farming", SkillCrafting: "Crafting", SkillDodge: "Dodge",
	SkillRide: "Ride",
}

// String is the game's enum name, or the number for one this package does
// not know.
func (t SkillType) String() string {
	if n, ok := skillNames[t]; ok {
		return n
	}
	return fmt.Sprintf("Skill(%d)", int32(t))
}

// SkillTypes lists every known skill in the game's numeric order.
func SkillTypes() []SkillType {
	out := make([]SkillType, 0, len(skillNames))
	for t := range skillNames {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ParseSkillType accepts the enum name in any case, with or without the
// underscore the console uses ("blood_magic"), or the number.
func ParseSkillType(s string) (SkillType, bool) {
	key := strings.ToLower(strings.ReplaceAll(s, "_", ""))
	for t, n := range skillNames {
		if strings.ToLower(n) == key {
			return t, true
		}
	}
	var n int32
	if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
		if _, ok := skillNames[SkillType(n)]; ok {
			return SkillType(n), true
		}
	}
	return 0, false
}

// Skill is one skill's state: the level and the progress toward the next.
type Skill struct {
	Type        SkillType
	Level       float32
	Accumulator float32
}

// Skills is Skills.Save's block, in file order.
type Skills struct {
	Version int32
	List    []Skill
}

// Get returns the skill, or nil when the character has never used it: the
// game only writes a skill once it has been raised.
func (s *Skills) Get(t SkillType) *Skill {
	for i := range s.List {
		if s.List[i].Type == t {
			return &s.List[i]
		}
	}
	return nil
}

// Set puts a skill at a level, adding it when absent, and clears the
// progress toward the next level since that progress was toward a level
// that is no longer the next one.
func (s *Skills) Set(t SkillType, level float32) {
	if sk := s.Get(t); sk != nil {
		sk.Level = level
		sk.Accumulator = 0
		return
	}
	s.List = append(s.List, Skill{Type: t, Level: level})
}

func (s *Skills) decode(r *zpackage.Reader) error {
	s.Version = r.ReadInt32()
	if s.Version != SkillsVersion {
		return fmt.Errorf("%w: skills %d", ErrVersion, s.Version)
	}
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1024 {
		return fmt.Errorf("character: %d skills", n)
	}
	s.List = make([]Skill, n)
	for i := range s.List {
		s.List[i].Type = SkillType(r.ReadInt32())
		s.List[i].Level = r.ReadSingle()
		s.List[i].Accumulator = r.ReadSingle()
	}
	if err := r.Err(); err != nil {
		return fmt.Errorf("character: skills: %w", err)
	}
	return nil
}

func (s *Skills) encode(w *zpackage.Writer) {
	w.WriteInt32(s.Version)
	w.WriteInt32(int32(len(s.List)))
	for _, sk := range s.List {
		w.WriteInt32(int32(sk.Type))
		w.WriteSingle(sk.Level)
		w.WriteSingle(sk.Accumulator)
	}
}
