package character

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Steam's app ID for Valheim, the folder name under userdata.
const steamAppID = "892970"

// Found is one character file on disk.
type Found struct {
	Path string
	// Name is the character as the file names it, lower case as the game
	// writes it; the profile holds the display name.
	Name string
	// Source says which kind of folder it came from: "cloud" for Steam's
	// synced store, "local" for the game's own folders.
	Source string
	Size   int64
}

// IsBackup reports whether the file is one of the game's backups rather than
// a live character: the game names them <name>_backup_<kind>-<stamp>.fch and
// keeps the previous save as <name>.fch.old.
func (f Found) IsBackup() bool {
	return strings.Contains(f.Name, "_backup_")
}

// Roots lists the folders a character file can live in on this machine, in
// the order the game prefers them: Steam Cloud's local copy first, then the
// game's own characters folder, then the local-only one.
//
// On Windows the game saves under AppData\LocalLow\IronGate\Valheim, and
// Steam keeps its cloud copy under userdata\<account>\892970\remote. Under
// WSL both are reached through /mnt/c. On Linux the game saves under
// ~/.config/unity3d and Steam under ~/.steam or ~/.local/share/Steam.
func Roots() []string {
	var roots []string
	add := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		roots = append(roots, matches...)
	}
	homes := windowsHomes()
	for _, h := range homes {
		add(filepath.Join(h, "AppData", "LocalLow", "IronGate", "Valheim"))
	}
	for _, s := range steamDirs() {
		add(filepath.Join(s, "userdata", "*", steamAppID, "remote"))
	}
	if runtime.GOOS != "windows" {
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".config", "unity3d", "IronGate", "Valheim"))
		}
	}
	return roots
}

func windowsHomes() []string {
	var out []string
	if runtime.GOOS == "windows" {
		if h, err := os.UserHomeDir(); err == nil {
			out = append(out, h)
		}
		return out
	}
	// WSL: every Windows user on the C drive, since the Linux user name need
	// not match the Windows one.
	matches, _ := filepath.Glob("/mnt/c/Users/*")
	for _, m := range matches {
		base := filepath.Base(m)
		switch base {
		case "Public", "Default", "Default User", "All Users":
			continue
		}
		out = append(out, m)
	}
	return out
}

func steamDirs() []string {
	var out []string
	if runtime.GOOS == "windows" {
		for _, pf := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			if pf != "" {
				out = append(out, filepath.Join(pf, "Steam"))
			}
		}
		return out
	}
	out = append(out, "/mnt/c/Program Files (x86)/Steam", "/mnt/c/Program Files/Steam")
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
		)
	}
	return out
}

// Locate finds every character file under Roots, backups included; filter
// with IsBackup. Files are returned in root order, then by name.
func Locate() []Found {
	var out []Found
	for _, root := range Roots() {
		source := "local"
		if strings.Contains(root, "userdata") {
			source = "cloud"
		}
		for _, sub := range []string{"characters", "characters_local"} {
			dir := filepath.Join(root, sub)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			var here []Found
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				here = append(here, Found{
					Path:   filepath.Join(dir, e.Name()),
					Name:   strings.TrimSuffix(e.Name(), Ext),
					Source: source,
					Size:   info.Size(),
				})
			}
			sort.Slice(here, func(i, j int) bool { return here[i].Name < here[j].Name })
			out = append(out, here...)
		}
	}
	return out
}

// Find returns the live character files whose name matches, case
// insensitively, across every root. More than one means the same character
// exists in two stores, which the caller must disambiguate.
func Find(name string) []Found {
	var out []Found
	for _, f := range Locate() {
		if f.IsBackup() {
			continue
		}
		if strings.EqualFold(f.Name, name) {
			out = append(out, f)
		}
	}
	return out
}
