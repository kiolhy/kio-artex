//go:build windows

package evidence

// Windows does not support fsync on directory handles. File contents are still
// flushed before the atomic rename; NTFS owns the directory metadata update.
func syncDir(string) error {
	return nil
}
