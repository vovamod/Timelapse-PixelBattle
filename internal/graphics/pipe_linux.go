//go:build linux

package graphics

import (
	"os"

	"golang.org/x/sys/unix"
)

func trySetPipeSize(f *os.File, size int) {
	_, _ = unix.FcntlInt(f.Fd(), unix.F_SETPIPE_SZ, size)
}
