package incident

import "strings"

func hasPathTraversal(path string) bool {
	return strings.Contains(path, ".."+string('/')) ||
		strings.Contains(path, "../") ||
		strings.Contains(path, `..\`) ||
		strings.Contains(path, ".."+string('\\'))
}
