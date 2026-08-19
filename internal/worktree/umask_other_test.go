//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || (js && wasm))

package worktree

// setUmask is a no-op on platforms without a umask. Tests that depend on mode
// bits skip themselves before calling it.
func setUmask(int) int { return 0 }
