package scheduler

import (
	"bytes"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// holdFile opens path so that others can only read it (no writing, renaming or
// deleting) until release is called, and checks that it holds want.
func holdFile(path string, want []byte) (release func(), err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	got, err := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
	if err == nil && !bytes.Equal(got, want) {
		err = errors.New("the task definition was changed before it was registered")
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
