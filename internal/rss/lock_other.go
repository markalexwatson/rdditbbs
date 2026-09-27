//go:build !unix

package rss

// LockSync has no cross-process lock on this platform; it always succeeds.
func LockSync(dir string) (unlock func(), ok bool, err error) {
	return func() {}, true, nil
}
