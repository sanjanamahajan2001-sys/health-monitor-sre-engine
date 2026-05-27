package disk

import (
	"fmt"
	"math"
	"syscall"
)

type DiskData struct {
	Used   string
	Total  string
	UsedPc int
}

func Collect() (DiskData, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return DiskData{}, err
	}
	total := float64(stat.Blocks) * float64(stat.Bsize)
	free := float64(stat.Bfree) * float64(stat.Bsize)
	used := total - free
	if total <= 0 {
		return DiskData{}, fmt.Errorf("invalid filesystem size for /")
	}
	pc := int(math.Round((used / total) * 100))

	return DiskData{
		Total:  formatBytes(total),
		Used:   formatBytes(used),
		UsedPc: pc,
	}, nil
}

func formatBytes(value float64) string {
	units := []string{"B", "K", "M", "G", "T", "P"}
	size := value
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if size >= 10 || unit == 0 {
		return fmt.Sprintf("%.0f%s", size, units[unit])
	}
	return fmt.Sprintf("%.1f%s", size, units[unit])
}
