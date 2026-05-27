package varfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"health-monitor/pkg/model"
)

func CollectTop() ([]model.VarItem, error) {
	var items []model.VarItem
	entries, err := os.ReadDir("/var")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join("/var", entry.Name())
		sizeMB := dirSizeMB(path)
		if sizeMB == 0 {
			continue
		}
		items = append(items, model.VarItem{
			Size: sizeMB,
			Path: path,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Size < items[j].Size
	})

	if len(items) <= 3 {
		return items, nil
	}
	return items[len(items)-3:], nil
}

func dirSizeMB(path string) int {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return int(info.Size() / (1024 * 1024))
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return int(total / (1024 * 1024))
}
