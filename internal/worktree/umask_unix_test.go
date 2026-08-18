//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || (js && wasm)

package worktree

import "syscall"

// setUmask sets the process umask and returns the previous value.
//
// The build constraint lists the platforms that provide syscall.Umask rather
// than using !windows: Plan 9 lacks it, so excluding only Windows would still
// fail to compile there.
func setUmask(mask int) int {
	return syscall.Umask(mask)
}
