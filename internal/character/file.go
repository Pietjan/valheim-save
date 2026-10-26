package character

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Ext is the character file's extension.
const Ext = ".fch"

// Load reads and decodes a character file.
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Save encodes the profile and writes it over path the way the game does:
// to a sibling first, then renamed into place, so a failure part way leaves
// the old file whole.
func Save(path string, p *Profile) error {
	data, err := p.Encode()
	if err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Backup copies the file beside itself as <name>_backup_edit-<stamp>.fch,
// the game's own backup naming with "edit" where it writes "auto", so the
// game's list of backups shows it and the next automatic one does not
// overwrite it. A backup is never overwritten either: a second edit within
// the same second gets a counter after the stamp. Returns the backup's path.
func Backup(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dir, file := filepath.Split(path)
	name := strings.TrimSuffix(file, filepath.Ext(file))
	stamp := time.Now().Format("20060102-150405")
	for n := 1; ; n++ {
		suffix := ""
		if n > 1 {
			suffix = fmt.Sprintf("-%d", n)
		}
		out := filepath.Join(dir, fmt.Sprintf("%s_backup_edit-%s%s%s", name, stamp, suffix, Ext))
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			os.Remove(out)
			return "", err
		}
		if err := f.Close(); err != nil {
			os.Remove(out)
			return "", err
		}
		return out, nil
	}
}
