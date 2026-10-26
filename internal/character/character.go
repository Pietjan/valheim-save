// Package character reads and writes Valheim character files (.fch).
//
// A character file is PlayerProfile.SavePlayerToDisk's output: a length-
// prefixed ZPackage holding the profile, followed by a length-prefixed SHA-512
// of that package. The game reads the hash and throws it away, so a file with
// a wrong hash loads fine, but one is written here all the same so that nothing
// downstream can tell an edited file from a saved one.
//
// Inside the profile the player's own state — health, inventory, skills,
// appearance, what they know — is a second ZPackage kept as a byte array,
// written by Player.Save. Both carry a version number, and this package reads
// exactly the layouts the 1.0.20 client writes (profile 47, player data 34),
// plus the one step older of each where the layout is the same but for a
// missing tail. Anything else is refused rather than guessed at: a field read
// at the wrong width is silently garbage from there to the end of the file,
// and the end of the file is where the skills are.
//
// Everything is decoded, and Encode writes it all back in the same order, so a
// file that is read and written with no change comes out byte for byte the
// same. The round trip over a real file is the test that the layout is right.
// Decoded structures keep their on-disk order: the game's dictionaries are
// written in insertion order and read back the same way, so they are slices of
// pairs here rather than maps.
package character

import (
	"crypto/sha512"
	"errors"
	"fmt"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// Profile versions this package reads. Version.Player in the game.
const (
	ProfileDeepNorth       = 46 // 1.0.17 to 1.0.19
	ProfileCheatCleanSlate = 47 // 1.0.20: same layout, the save marks older files for backup
)

// ErrVersion is wrapped by every refusal to read a layout this package does
// not know.
var ErrVersion = errors.New("character: unsupported version")

// Profile is one character file.
type Profile struct {
	Version int32

	// Stats is one table per difficulty category, ten of them, each a row of
	// PlayerStatType counters followed by the named tallies.
	Stats []StatTable

	FirstSpawn bool

	// Worlds is the per-world state, keyed by the world's UID.
	Worlds []WorldData

	Name        string
	PlayerID    int64
	StartSeed   string
	UsedCheats  bool
	DateCreated int64 // unix seconds

	// Player is the player block, nil when the file has none (a character
	// made but never spawned).
	Player *Player
}

// StatTable is one difficulty category's statistics.
type StatTable struct {
	Counters     []float32 // indexed by PlayerStatType
	KnownWorlds  []NamedFloat
	KnownKeys    []NamedFloat
	Commands     []NamedFloat
	EnemyStats   [][]NamedFloat // one list per enemy stat kind, five of them
	ItemPickups  []NamedFloat
	ItemCrafts   []NamedFloat
	Pickables    []NamedFloat
	FoodEaten    []NamedFloat
	PiecesPlaced []NamedFloat
}

// NamedFloat is one entry of a string-keyed tally.
type NamedFloat struct {
	Name  string
	Value float32
}

// WorldData is what the profile keeps about one world.
type WorldData struct {
	Key             int64
	HaveCustomSpawn bool
	SpawnPoint      zpackage.Vector3
	HaveLogoutPoint bool
	LogoutPoint     zpackage.Vector3
	HaveDeathPoint  bool
	DeathPoint      zpackage.Vector3
	HomePoint       zpackage.Vector3
	// MapData is the minimap as the game wrote it, nil when the world has
	// none. It is kept raw because it is gzip inside, and recompressing it
	// would change bytes for no reason; decode it with Map and put a changed
	// one back with SetMap.
	MapData []byte
}

// Decode parses a character file: the outer length-prefixed package and its
// hash, then the profile and the player block inside it.
func Decode(file []byte) (*Profile, error) {
	outer := zpackage.NewReader(file)
	data := outer.ReadByteArray()
	hash := outer.ReadByteArray()
	if err := outer.Err(); err != nil {
		return nil, fmt.Errorf("character: file framing: %w", err)
	}
	if outer.Remaining() != 0 {
		return nil, fmt.Errorf("character: %d bytes after the hash", outer.Remaining())
	}
	if len(hash) != sha512.Size {
		return nil, fmt.Errorf("character: hash is %d bytes, want %d", len(hash), sha512.Size)
	}
	// The hash is informational, as it is to the game: a mismatch is reported
	// through Verify and is not a reason to refuse the file.
	return decodeProfile(data)
}

// Verify reports whether the file's hash matches its contents.
func Verify(file []byte) bool {
	outer := zpackage.NewReader(file)
	data := outer.ReadByteArray()
	hash := outer.ReadByteArray()
	if outer.Err() != nil {
		return false
	}
	sum := sha512.Sum512(data)
	return string(sum[:]) == string(hash)
}

// Encode writes the profile as a character file, with a fresh hash.
func (p *Profile) Encode() ([]byte, error) {
	data, err := p.encodeProfile()
	if err != nil {
		return nil, err
	}
	sum := sha512.Sum512(data)
	w := zpackage.NewWriter()
	w.WriteByteArray(data)
	w.WriteByteArray(sum[:])
	return w.Bytes(), nil
}

func decodeProfile(data []byte) (*Profile, error) {
	r := zpackage.NewReader(data)
	p := &Profile{}
	p.Version = r.ReadInt32()
	if p.Version != ProfileDeepNorth && p.Version != ProfileCheatCleanSlate {
		return nil, fmt.Errorf("%w: profile %d", ErrVersion, p.Version)
	}
	numStats := int(r.ReadInt32())
	numTables := int(r.ReadInt32())
	if r.Err() != nil || numStats < 0 || numStats > 4096 || numTables < 0 || numTables > 64 {
		return nil, fmt.Errorf("character: stat table header %d x %d", numStats, numTables)
	}
	p.Stats = make([]StatTable, numTables)
	for i := range p.Stats {
		t := &p.Stats[i]
		t.Counters = make([]float32, numStats)
		for j := range t.Counters {
			t.Counters[j] = r.ReadSingle()
		}
		t.KnownWorlds = readNamedFloats(r)
		t.KnownKeys = readNamedFloats(r)
		t.Commands = readNamedFloats(r)
		n := int(r.ReadInt32())
		if r.Err() != nil || n < 0 || n > 64 {
			return nil, fmt.Errorf("character: %d enemy stat kinds", n)
		}
		t.EnemyStats = make([][]NamedFloat, n)
		for k := range t.EnemyStats {
			t.EnemyStats[k] = readNamedFloats(r)
		}
		t.ItemPickups = readNamedFloats(r)
		t.ItemCrafts = readNamedFloats(r)
		t.Pickables = readNamedFloats(r)
		t.FoodEaten = readNamedFloats(r)
		t.PiecesPlaced = readNamedFloats(r)
		if err := r.Err(); err != nil {
			return nil, fmt.Errorf("character: stats table %d: %w", i, err)
		}
	}
	p.FirstSpawn = r.ReadBool()
	nw := int(r.ReadInt32())
	if r.Err() != nil || nw < 0 || nw > 100000 {
		return nil, fmt.Errorf("character: %d worlds", nw)
	}
	p.Worlds = make([]WorldData, nw)
	for i := range p.Worlds {
		w := &p.Worlds[i]
		w.Key = r.ReadInt64()
		w.HaveCustomSpawn = r.ReadBool()
		w.SpawnPoint = r.ReadVector3()
		w.HaveLogoutPoint = r.ReadBool()
		w.LogoutPoint = r.ReadVector3()
		w.HaveDeathPoint = r.ReadBool()
		w.DeathPoint = r.ReadVector3()
		w.HomePoint = r.ReadVector3()
		if r.ReadBool() {
			w.MapData = r.ReadByteArray()
		}
		if err := r.Err(); err != nil {
			return nil, fmt.Errorf("character: world %d: %w", i, err)
		}
	}
	p.Name = r.ReadString()
	p.PlayerID = r.ReadInt64()
	p.StartSeed = r.ReadString()
	p.UsedCheats = r.ReadBool()
	p.DateCreated = r.ReadInt64()
	var playerData []byte
	if r.ReadBool() {
		playerData = r.ReadByteArray()
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("character: profile tail: %w", err)
	}
	if r.Remaining() != 0 {
		return nil, fmt.Errorf("character: %d bytes after the player data", r.Remaining())
	}
	if playerData != nil {
		pl, err := decodePlayer(playerData)
		if err != nil {
			return nil, err
		}
		p.Player = pl
	}
	return p, nil
}

func (p *Profile) encodeProfile() ([]byte, error) {
	if p.Version != ProfileDeepNorth && p.Version != ProfileCheatCleanSlate {
		return nil, fmt.Errorf("%w: profile %d", ErrVersion, p.Version)
	}
	w := zpackage.NewWriter()
	w.WriteInt32(p.Version)
	numStats := 0
	if len(p.Stats) > 0 {
		numStats = len(p.Stats[0].Counters)
	}
	w.WriteInt32(int32(numStats))
	w.WriteInt32(int32(len(p.Stats)))
	for i, t := range p.Stats {
		if len(t.Counters) != numStats {
			return nil, fmt.Errorf("character: stats table %d has %d counters, table 0 has %d", i, len(t.Counters), numStats)
		}
		for _, v := range t.Counters {
			w.WriteSingle(v)
		}
		writeNamedFloats(w, t.KnownWorlds)
		writeNamedFloats(w, t.KnownKeys)
		writeNamedFloats(w, t.Commands)
		w.WriteInt32(int32(len(t.EnemyStats)))
		for _, e := range t.EnemyStats {
			writeNamedFloats(w, e)
		}
		writeNamedFloats(w, t.ItemPickups)
		writeNamedFloats(w, t.ItemCrafts)
		writeNamedFloats(w, t.Pickables)
		writeNamedFloats(w, t.FoodEaten)
		writeNamedFloats(w, t.PiecesPlaced)
	}
	w.WriteBool(p.FirstSpawn)
	w.WriteInt32(int32(len(p.Worlds)))
	for _, wd := range p.Worlds {
		w.WriteInt64(wd.Key)
		w.WriteBool(wd.HaveCustomSpawn)
		w.WriteVector3(wd.SpawnPoint)
		w.WriteBool(wd.HaveLogoutPoint)
		w.WriteVector3(wd.LogoutPoint)
		w.WriteBool(wd.HaveDeathPoint)
		w.WriteVector3(wd.DeathPoint)
		w.WriteVector3(wd.HomePoint)
		w.WriteBool(wd.MapData != nil)
		if wd.MapData != nil {
			w.WriteByteArray(wd.MapData)
		}
	}
	w.WriteString(p.Name)
	w.WriteInt64(p.PlayerID)
	w.WriteString(p.StartSeed)
	w.WriteBool(p.UsedCheats)
	w.WriteInt64(p.DateCreated)
	w.WriteBool(p.Player != nil)
	if p.Player != nil {
		data, err := p.Player.encode()
		if err != nil {
			return nil, err
		}
		w.WriteByteArray(data)
	}
	return w.Bytes(), nil
}

func readNamedFloats(r *zpackage.Reader) []NamedFloat {
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1<<20 {
		r.SetPos(r.Size() + 1) // force the sticky overflow error
		return nil
	}
	out := make([]NamedFloat, n)
	for i := range out {
		out[i].Name = r.ReadString()
		out[i].Value = r.ReadSingle()
	}
	return out
}

func writeNamedFloats(w *zpackage.Writer, v []NamedFloat) {
	w.WriteInt32(int32(len(v)))
	for _, e := range v {
		w.WriteString(e.Name)
		w.WriteSingle(e.Value)
	}
}

func readStrings(r *zpackage.Reader) []string {
	n := int(r.ReadInt32())
	if r.Err() != nil || n < 0 || n > 1<<20 {
		r.SetPos(r.Size() + 1)
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = r.ReadString()
	}
	return out
}

func writeStrings(w *zpackage.Writer, v []string) {
	w.WriteInt32(int32(len(v)))
	for _, s := range v {
		w.WriteString(s)
	}
}
