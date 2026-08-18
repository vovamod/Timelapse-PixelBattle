//go:build !linux

package graphics

import "os"

func trySetPipeSize(f *os.File, size int) {}
