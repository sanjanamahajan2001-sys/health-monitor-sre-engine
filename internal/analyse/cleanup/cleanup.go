package cleanup

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type Result struct {
	DiskUsagePct int
	VarLogMB     int
	VarCacheMB   int
}

func Collect() (Result, error) {
	diskPct, err := diskUsagePct("/")
	if err != nil {
		return Result{}, err
	}
	logMB := dirSizeMB("/var/log")
	cacheMB := dirSizeMB("/var/cache")

	return Result{
		DiskUsagePct: diskPct,
		VarLogMB:     logMB,
		VarCacheMB:   cacheMB,
	}, nil
}

func diskUsagePct(path string) (int, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := float64(stat.Blocks) * float64(stat.Bsize)
	free := float64(stat.Bfree) * float64(stat.Bsize)
	used := total - free
	if total <= 0 {
		return 0, fmt.Errorf("invalid filesystem size for %s", path)
	}
	return int((used / total) * 100), nil
}

func dirSizeMB(path string) int {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return 0
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
