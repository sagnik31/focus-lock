//go:build !windows

package scheduler

func holdFile(string, []byte) (release func(), err error) { return func() {}, nil }
