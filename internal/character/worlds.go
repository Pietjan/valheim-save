package character

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pietjan/valheim-save/internal/zpackage"
)

// The profile keys its per-world data by the world's UID and never writes the
// world's name next to it; the names live in the world files. A world's
// metadata is a small length-prefixed package (.fwl, or _main.<n>.fwl2 inside
// a folder named after the world since saves were chunked) holding the
// version, the name, the seed name, the seed and the UID. Reading those from
// the same roots the characters come from pairs most keys with a name. A
// world played only on somebody else's server has no local file and stays a
// number.

// WorldName is one world file's identity.
type WorldName struct {
	Key  int64
	Name string
	Seed string
	Path string
}

var chunkedMeta = regexp.MustCompile(`^_main\.(\d+)\.fwl2$`)

// WorldNames reads every world file under Roots, and under any extra
// folders given (a server's worlds_local, say), and maps UID to name. The
// newest chunk of a chunked save wins; a plain .fwl beside a folder of the
// same name is the older of the two and loses.
func WorldNames(extra ...string) map[int64]WorldName {
	out := map[int64]WorldName{}
	var dirs []string
	for _, root := range Roots() {
		dirs = append(dirs, filepath.Join(root, "worlds"), filepath.Join(root, "worlds_local"))
	}
	dirs = append(dirs, extra...)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.Contains(name, "_backup_") {
				continue
			}
			var path string
			switch {
			case e.IsDir():
				path = newestChunk(filepath.Join(dir, name))
			case strings.HasSuffix(name, ".fwl"):
				path = filepath.Join(dir, name)
			}
			if path == "" {
				continue
			}
			w, ok := readWorldMeta(path)
			if !ok {
				continue
			}
			if prev, dup := out[w.Key]; dup && !e.IsDir() && strings.HasSuffix(prev.Path, ".fwl2") {
				continue
			}
			out[w.Key] = w
		}
	}
	return out
}

func newestChunk(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best, bestN := "", -1
	for _, e := range entries {
		m := chunkedMeta.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n > bestN {
			best, bestN = filepath.Join(dir, e.Name()), n
		}
	}
	return best
}

func readWorldMeta(path string) (WorldName, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorldName{}, false
	}
	outer := zpackage.NewReader(data)
	r := outer.ReadPackage()
	if outer.Err() != nil {
		return WorldName{}, false
	}
	r.ReadInt32() // version; the four fields read here have not moved since 2021
	w := WorldName{Path: path}
	w.Name = r.ReadString()
	w.Seed = r.ReadString()
	r.ReadInt32() // seed
	w.Key = r.ReadInt64()
	if r.Err() != nil || w.Name == "" {
		return WorldName{}, false
	}
	return w, true
}

// SortedWorldNames is WorldNames as a list, by name.
func SortedWorldNames(extra ...string) []WorldName {
	m := WorldNames(extra...)
	out := make([]WorldName, 0, len(m))
	for _, w := range m {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
