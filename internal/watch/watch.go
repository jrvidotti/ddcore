// Package watch polls app directories and triggers reloads (no fsnotify dependency).
package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrvidotti/cerne/internal/engine"
)

func snapshot(dirs []string) map[string]int64 {
	m := map[string]int64{}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".csv") {
				if info, err := d.Info(); err == nil {
					m[p] = info.ModTime().UnixNano() + info.Size()
				}
			}
			return nil
		})
	}
	return m
}

// Apps calls onChange whenever a .ts/.csv file in any app changes.
func Apps(ctx context.Context, e *engine.Engine, onChange func()) {
	var dirs []string
	for _, a := range e.Apps {
		if a.Embedded == nil {
			dirs = append(dirs, a.Dir)
		}
	}
	last := snapshot(dirs)
	t := time.NewTicker(700 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cur := snapshot(dirs)
			if !same(last, cur) {
				last = cur
				// debounce: wait for writes to settle
				time.Sleep(150 * time.Millisecond)
				last = snapshot(dirs)
				e.Log.Info("mudança detectada, recarregando apps")
				onChange()
			}
		}
	}
}

func same(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
