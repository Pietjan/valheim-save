package character

import (
	"fmt"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// Map data versions this package reads. Version.Map in the game. Older maps
// are uncompressed and written as one bool per cell; nothing has written one
// since 2023 and the game converts them on its next save, so they are refused.
const (
	MapCompressed = 7
	MapPinsAuthor = 8 // 1.0.x: pins carry their author's platform ID
)

// Map is a world's minimap as Minimap.GetMapData writes it: the fog of war as
// one byte per cell, twice (what this player explored, and what was shared by
// others), the saved pins, and whether the player's position is public.
type Map struct {
	Version int32
	// Size is the map texture's edge; the game's is 2048 and it refuses a map
	// of any other size, so a Map is never resized here.
	Size           int32
	Explored       []byte // Size*Size, row-major, 0 or 1
	ExploredOthers []byte
	Pins           []Pin
	PublicPosition bool
}

// Pin is a saved map marker.
type Pin struct {
	Name    string
	Pos     zpackage.Vector3
	Type    PinType
	Checked bool
	OwnerID int64
	Author  string // platform user ID, empty for one's own pins before authors were recorded
}

// PinType is Minimap.PinType in the game.
type PinType int32

var pinNames = []string{"Icon0", "Icon1", "Icon2", "Icon3", "Death", "Bed", "Icon4", "Shout", "None", "Boss", "Player", "RandomEvent", "Ping", "EventArea", "Hildir1", "Hildir2", "Hildir3", "Memorial"}

func (t PinType) String() string {
	if int(t) >= 0 && int(t) < len(pinNames) {
		return pinNames[t]
	}
	return fmt.Sprintf("Pin(%d)", int32(t))
}

// Map decodes the world's map data, or returns nil when the world has none.
func (w *WorldData) Map() (*Map, error) {
	if w.MapData == nil {
		return nil, nil
	}
	return decodeMap(w.MapData)
}

// SetMap replaces the world's map data with m encoded.
func (w *WorldData) SetMap(m *Map) error {
	data, err := m.encode()
	if err != nil {
		return err
	}
	w.MapData = data
	return nil
}

// ExploredCount is how many cells of the player's own fog are lifted.
func (m *Map) ExploredCount() int {
	n := 0
	for _, b := range m.Explored {
		if b != 0 {
			n++
		}
	}
	return n
}

// Reveal lifts the fog everywhere.
func (m *Map) Reveal() {
	for i := range m.Explored {
		m.Explored[i] = 1
	}
}

// Wipe puts the fog back everywhere, the player's own and what was shared.
func (m *Map) Wipe() {
	for i := range m.Explored {
		m.Explored[i] = 0
		m.ExploredOthers[i] = 0
	}
}

func decodeMap(data []byte) (*Map, error) {
	outer := zpackage.NewReader(data)
	m := &Map{}
	m.Version = outer.ReadInt32()
	if m.Version != MapCompressed && m.Version != MapPinsAuthor {
		return nil, fmt.Errorf("%w: map %d", ErrVersion, m.Version)
	}
	r := outer.ReadCompressedPackage()
	if err := outer.Err(); err != nil {
		return nil, fmt.Errorf("character: map: %w", err)
	}
	if outer.Remaining() != 0 {
		return nil, fmt.Errorf("character: %d bytes after the map package", outer.Remaining())
	}
	m.Size = r.ReadInt32()
	if r.Err() != nil || m.Size <= 0 || m.Size > 8192 {
		return nil, fmt.Errorf("character: map size %d", m.Size)
	}
	cells := int(m.Size) * int(m.Size)
	m.Explored = r.ReadRawBytes(cells)
	m.ExploredOthers = r.ReadRawBytes(cells)
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1<<20 {
		return nil, fmt.Errorf("character: %d pins", n)
	}
	m.Pins = make([]Pin, n)
	for i := range m.Pins {
		p := &m.Pins[i]
		p.Name = r.ReadString()
		p.Pos = r.ReadVector3()
		p.Type = PinType(r.ReadInt32())
		p.Checked = r.ReadBool()
		p.OwnerID = r.ReadInt64()
		if m.Version >= MapPinsAuthor {
			p.Author = r.ReadString()
		}
	}
	m.PublicPosition = r.ReadBool()
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("character: map body: %w", err)
	}
	if r.Remaining() != 0 {
		return nil, fmt.Errorf("character: %d bytes after the map body", r.Remaining())
	}
	return m, nil
}

func (m *Map) encode() ([]byte, error) {
	if m.Version != MapCompressed && m.Version != MapPinsAuthor {
		return nil, fmt.Errorf("%w: map %d", ErrVersion, m.Version)
	}
	cells := int(m.Size) * int(m.Size)
	if len(m.Explored) != cells || len(m.ExploredOthers) != cells {
		return nil, fmt.Errorf("character: map of size %d has %d and %d cells", m.Size, len(m.Explored), len(m.ExploredOthers))
	}
	body := zpackage.NewWriter()
	body.WriteInt32(m.Size)
	body.WriteRaw(m.Explored)
	body.WriteRaw(m.ExploredOthers)
	body.WriteInt32(int32(len(m.Pins)))
	for _, p := range m.Pins {
		body.WriteString(p.Name)
		body.WriteVector3(p.Pos)
		body.WriteInt32(int32(p.Type))
		body.WriteBool(p.Checked)
		body.WriteInt64(p.OwnerID)
		if m.Version >= MapPinsAuthor {
			body.WriteString(p.Author)
		}
	}
	body.WriteBool(m.PublicPosition)
	w := zpackage.NewWriter()
	w.WriteInt32(m.Version)
	if err := w.WriteCompressedPackage(body); err != nil {
		return nil, fmt.Errorf("character: map: %w", err)
	}
	return w.Bytes(), nil
}
