//go:build !windows

package localstate

import "os"

func replaceFile(sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}
