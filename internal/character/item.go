package character

import (
	"fmt"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// Inventory versions this package reads. Version.Item in the game.
const (
	ItemChunksNCheats = 109 // 1.0.17 to 1.0.19
	ItemNewCheats     = 110 // 1.0.20: same layout; the cheated bit is only honoured from here
)

// Inventory is Inventory.Save's block: a version, then the items.
type Inventory struct {
	Version int32
	Items   []Item
}

// Item is one inventory slot as ItemDrop.ItemData.Save writes it.
//
// The game writes a flags byte it derives from the values (quality other than
// one, stack other than one, and so on) and only then the optional fields the
// flags announce. Encode derives the flags the same way, so changing Quality
// to 1 drops the quality field as the game would. HasPrefab is the one flag
// that is not derived from another field here — it is whether the item had a
// drop prefab, which every real item does — and is kept as read.
type Item struct {
	// DurabilityCenti is durability in hundredths, the integer the game
	// stores; Durability gives it as the float the tooltip shows.
	DurabilityCenti int32
	GridX, GridY    uint8
	WorldLevel      uint8
	PickedUp        bool
	Equipped        bool
	Quality         uint16
	Stack           uint16
	Variant         int32
	CrafterID       int64
	CrafterName     string
	HasPrefab       bool
	PrefabHash      int32
	CustomData      []NamedString
	Cheated         bool
}

// Durability is the item's durability as the game shows it.
func (it Item) Durability() float32 { return float32(it.DurabilityCenti) / 100 }

func (inv *Inventory) decode(r *zpackage.Reader) error {
	inv.Version = r.ReadInt32()
	if inv.Version != ItemChunksNCheats && inv.Version != ItemNewCheats {
		return fmt.Errorf("%w: inventory %d", ErrVersion, inv.Version)
	}
	n := int(r.ReadUInt16())
	if r.Err() != nil {
		return fmt.Errorf("character: inventory head: %w", r.Err())
	}
	inv.Items = make([]Item, n)
	for i := range inv.Items {
		it := &inv.Items[i]
		it.DurabilityCenti = r.ReadInt32()
		it.GridX = r.ReadUInt8()
		it.GridY = r.ReadUInt8()
		it.WorldLevel = r.ReadUInt8()
		flags := r.ReadUInt8()
		it.PickedUp = flags&1 != 0
		it.Equipped = flags&2 != 0
		it.Quality, it.Stack = 1, 1
		if flags&4 != 0 {
			it.Quality = r.ReadUInt16()
		}
		if flags&8 != 0 {
			it.Stack = r.ReadUInt16()
		}
		if flags&0x10 != 0 {
			it.Variant = r.ReadInt32()
		}
		if flags&0x20 != 0 {
			it.CrafterID = r.ReadInt64()
			it.CrafterName = r.ReadString()
		}
		if flags&0x40 != 0 {
			it.HasPrefab = true
			it.PrefabHash = r.ReadInt32()
		}
		if flags&0x80 != 0 {
			cn := r.ReadNumItems()
			it.CustomData = make([]NamedString, cn)
			for k := range it.CustomData {
				it.CustomData[k].Name = r.ReadString()
				it.CustomData[k].Value = r.ReadString()
			}
		}
		it.Cheated = r.ReadUInt8()&1 != 0
		if err := r.Err(); err != nil {
			return fmt.Errorf("character: inventory item %d: %w", i, err)
		}
		// The writer's rule, checked on the way in so a file this package
		// would not reproduce is refused rather than silently rewritten.
		if (flags&4 != 0) != (it.Quality != 1) || (flags&8 != 0) != (it.Stack != 1) ||
			(flags&0x10 != 0) != (it.Variant != 0) || (flags&0x20 != 0) != (it.CrafterID != 0) ||
			(flags&0x80 != 0) != (len(it.CustomData) != 0) {
			return fmt.Errorf("character: inventory item %d: flags %#x disagree with its values", i, flags)
		}
	}
	return nil
}

func (inv *Inventory) encode(w *zpackage.Writer) error {
	if inv.Version != ItemChunksNCheats && inv.Version != ItemNewCheats {
		return fmt.Errorf("%w: inventory %d", ErrVersion, inv.Version)
	}
	if len(inv.Items) > 0xFFFF {
		return fmt.Errorf("character: %d inventory items", len(inv.Items))
	}
	w.WriteInt32(inv.Version)
	w.WriteUInt16(uint16(len(inv.Items)))
	for _, it := range inv.Items {
		var flags uint8
		if it.PickedUp {
			flags |= 1
		}
		if it.Equipped {
			flags |= 2
		}
		if it.Quality != 1 {
			flags |= 4
		}
		if it.Stack != 1 {
			flags |= 8
		}
		if it.Variant != 0 {
			flags |= 0x10
		}
		if it.CrafterID != 0 {
			flags |= 0x20
		}
		if it.HasPrefab {
			flags |= 0x40
		}
		if len(it.CustomData) != 0 {
			flags |= 0x80
		}
		w.WriteInt32(it.DurabilityCenti)
		w.WriteUInt8(it.GridX)
		w.WriteUInt8(it.GridY)
		w.WriteUInt8(it.WorldLevel)
		w.WriteUInt8(flags)
		if flags&4 != 0 {
			w.WriteUInt16(it.Quality)
		}
		if flags&8 != 0 {
			w.WriteUInt16(it.Stack)
		}
		if flags&0x10 != 0 {
			w.WriteInt32(it.Variant)
		}
		if flags&0x20 != 0 {
			w.WriteInt64(it.CrafterID)
			w.WriteString(it.CrafterName)
		}
		if flags&0x40 != 0 {
			w.WriteInt32(it.PrefabHash)
		}
		if flags&0x80 != 0 {
			w.WriteNumItems(len(it.CustomData))
			for _, c := range it.CustomData {
				w.WriteString(c.Name)
				w.WriteString(c.Value)
			}
		}
		var cheat uint8
		if it.Cheated {
			cheat = 1
		}
		w.WriteUInt8(cheat)
	}
	return nil
}
