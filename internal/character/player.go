package character

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// Player data versions this package reads. Version.PlayerData in the game.
const (
	PlayerChunkedNorth = 33 // 1.0.17 to 1.0.19
	PlayerPostLaunch   = 34 // 1.0.20: adds the drop counters and upgrade seeds
)

// Player is the player block: Player.Save's package inside the profile.
type Player struct {
	Version int32

	MaxHealth             float32
	Health                float32
	MaxStamina            float32
	TimeSinceDeath        float32
	GuardianPower         string
	GuardianPowerCooldown float32

	Inventory Inventory

	KnownRecipes   []string
	KnownStations  []NamedInt
	KnownMaterials []string
	ShownTutorials []string
	Uniques        []string
	Trophies       []string
	KnownBiomes    []string
	KnownTexts     []NamedString

	Beard     string
	Hair      string
	SkinColor zpackage.Vector3
	HairColor zpackage.Vector3
	Model     int32 // 0 male body, 1 female body

	Foods []Food

	Skills Skills

	CustomData []NamedString

	Stamina float32
	MaxEitr float32
	Eitr    float32

	// BuildUI is the hammer's menu state (Hud.m_buildUi.SaveToBinary), opaque.
	BuildUI []byte

	// Only from PlayerPostLaunch on.
	DropCounters []IntPair // CharacterDrop.s_pseudoCounter
	UpgradeSeeds []IntPair // Player.m_itemUpgradeSeeds
}

// NamedInt is one entry of a string-keyed int table.
type NamedInt struct {
	Name  string
	Value int32
}

// NamedString is one entry of a string-keyed string table.
type NamedString struct {
	Name  string
	Value string
}

// IntPair is one entry of an int-keyed int table.
type IntPair struct {
	Key   int32
	Value int32
}

// Food is one of the three food slots.
type Food struct {
	Name string
	Time float32 // seconds left
}

func decodePlayer(data []byte) (*Player, error) {
	r := zpackage.NewReader(data)
	p := &Player{}
	p.Version = r.ReadInt32()
	if p.Version != PlayerChunkedNorth && p.Version != PlayerPostLaunch {
		return nil, fmt.Errorf("%w: player data %d", ErrVersion, p.Version)
	}
	p.MaxHealth = r.ReadSingle()
	p.Health = r.ReadSingle()
	p.MaxStamina = r.ReadSingle()
	p.TimeSinceDeath = r.ReadSingle()
	p.GuardianPower = r.ReadString()
	p.GuardianPowerCooldown = r.ReadSingle()
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("character: player head: %w", err)
	}
	if err := p.Inventory.decode(r); err != nil {
		return nil, err
	}
	p.KnownRecipes = readStrings(r)
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1<<16 {
		return nil, fmt.Errorf("character: %d known stations", n)
	}
	p.KnownStations = make([]NamedInt, n)
	for i := range p.KnownStations {
		p.KnownStations[i].Name = r.ReadString()
		p.KnownStations[i].Value = r.ReadInt32()
	}
	p.KnownMaterials = readStrings(r)
	p.ShownTutorials = readStrings(r)
	p.Uniques = readStrings(r)
	p.Trophies = readStrings(r)
	p.KnownBiomes = readStrings(r)
	p.KnownTexts = readNamedStrings(r)
	p.Beard = r.ReadString()
	p.Hair = r.ReadString()
	p.SkinColor = r.ReadVector3()
	p.HairColor = r.ReadVector3()
	p.Model = r.ReadInt32()
	n = int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 64 {
		return nil, fmt.Errorf("character: %d foods", n)
	}
	p.Foods = make([]Food, n)
	for i := range p.Foods {
		p.Foods[i].Name = r.ReadString()
		p.Foods[i].Time = r.ReadSingle()
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("character: player knowledge: %w", err)
	}
	if err := p.Skills.decode(r); err != nil {
		return nil, err
	}
	p.CustomData = readNamedStrings(r)
	p.Stamina = r.ReadSingle()
	p.MaxEitr = r.ReadSingle()
	p.Eitr = r.ReadSingle()
	p.BuildUI = r.ReadByteArray()
	if p.BuildUI == nil {
		p.BuildUI = []byte{}
	}
	if p.Version >= PlayerPostLaunch {
		p.DropCounters = readIntPairs(r)
		p.UpgradeSeeds = readIntPairs(r)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("character: player tail: %w", err)
	}
	if r.Remaining() != 0 {
		return nil, fmt.Errorf("character: %d bytes after the player block", r.Remaining())
	}
	return p, nil
}

func (p *Player) encode() ([]byte, error) {
	if p.Version != PlayerChunkedNorth && p.Version != PlayerPostLaunch {
		return nil, fmt.Errorf("%w: player data %d", ErrVersion, p.Version)
	}
	w := zpackage.NewWriter()
	w.WriteInt32(p.Version)
	w.WriteSingle(p.MaxHealth)
	w.WriteSingle(p.Health)
	w.WriteSingle(p.MaxStamina)
	w.WriteSingle(p.TimeSinceDeath)
	w.WriteString(p.GuardianPower)
	w.WriteSingle(p.GuardianPowerCooldown)
	if err := p.Inventory.encode(w); err != nil {
		return nil, err
	}
	writeStrings(w, p.KnownRecipes)
	w.WriteInt32(int32(len(p.KnownStations)))
	for _, s := range p.KnownStations {
		w.WriteString(s.Name)
		w.WriteInt32(s.Value)
	}
	writeStrings(w, p.KnownMaterials)
	writeStrings(w, p.ShownTutorials)
	writeStrings(w, p.Uniques)
	writeStrings(w, p.Trophies)
	writeStrings(w, p.KnownBiomes)
	writeNamedStrings(w, p.KnownTexts)
	w.WriteString(p.Beard)
	w.WriteString(p.Hair)
	w.WriteVector3(p.SkinColor)
	w.WriteVector3(p.HairColor)
	w.WriteInt32(p.Model)
	w.WriteInt32(int32(len(p.Foods)))
	for _, f := range p.Foods {
		w.WriteString(f.Name)
		w.WriteSingle(f.Time)
	}
	p.Skills.encode(w)
	writeNamedStrings(w, p.CustomData)
	w.WriteSingle(p.Stamina)
	w.WriteSingle(p.MaxEitr)
	w.WriteSingle(p.Eitr)
	w.WriteByteArray(p.BuildUI)
	if p.Version >= PlayerPostLaunch {
		writeIntPairs(w, p.DropCounters)
		writeIntPairs(w, p.UpgradeSeeds)
	}
	return w.Bytes(), nil
}

func readNamedStrings(r *zpackage.Reader) []NamedString {
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1<<20 {
		r.SetPos(r.Size() + 1)
		return nil
	}
	out := make([]NamedString, n)
	for i := range out {
		out[i].Name = r.ReadString()
		out[i].Value = r.ReadString()
	}
	return out
}

func writeNamedStrings(w *zpackage.Writer, v []NamedString) {
	w.WriteInt32(int32(len(v)))
	for _, e := range v {
		w.WriteString(e.Name)
		w.WriteString(e.Value)
	}
}

func readIntPairs(r *zpackage.Reader) []IntPair {
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1<<20 {
		r.SetPos(r.Size() + 1)
		return nil
	}
	out := make([]IntPair, n)
	for i := range out {
		out[i].Key = r.ReadInt32()
		out[i].Value = r.ReadInt32()
	}
	return out
}

func writeIntPairs(w *zpackage.Writer, v []IntPair) {
	w.WriteInt32(int32(len(v)))
	for _, e := range v {
		w.WriteInt32(e.Key)
		w.WriteInt32(e.Value)
	}
}

// InventoryWidth is the player inventory's columns; the rows are bought.
const InventoryWidth = 8

// InventoryRows is how many rows the inventory has: four to start, more as
// bought from the trader, kept as the unique key "invrows <n>".
func (p *Player) InventoryRows() int {
	for _, u := range p.Uniques {
		if v, ok := strings.CutPrefix(u, "invrows "); ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
	}
	return 4
}

// FreeSlots is how many inventory slots hold nothing.
func (p *Player) FreeSlots() int {
	return InventoryWidth*p.InventoryRows() - len(p.Inventory.Items)
}
