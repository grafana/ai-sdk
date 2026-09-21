package service

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPhysicalSocketOutput_WriteDeadlineAndClose(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer func() { _ = unix.Close(fds[1]) }()
	if runtime.GOOS != "linux" {
		require.NoError(t, unix.SetNonblock(fds[0], true))
	}
	output := &physicalSocketOutput{fd: fds[0]}
	defer func() { _ = output.Close() }()
	require.NoError(t, output.SetWriteDeadline(time.Now().Add(time.Second)))
	n, err := output.Write([]byte("record\n"))
	require.NoError(t, err)
	require.Equal(t, 7, n)
	buffer := make([]byte, 7)
	n, err = unix.Read(fds[1], buffer)
	require.NoError(t, err)
	assert.Equal(t, "record\n", string(buffer[:n]))
	require.NoError(t, output.SetWriteDeadline(time.Now().Add(20*time.Millisecond)))
	started := time.Now()
	n, err = output.Write(make([]byte, 8<<20))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	assert.Less(t, n, 8<<20)
	assert.Less(t, time.Since(started), time.Second)
}

func TestPhysicalSocketOutput_BlockingDescriptorPolicyPreservesFlags(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer func() { _ = unix.Close(fds[0]) }()
	defer func() { _ = unix.Close(fds[1]) }()
	before, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFL, 0)
	require.NoError(t, err)
	if runtime.GOOS == "linux" {
		require.NoError(t, physicalSocketDeadlineSupported(fds[0]))
	} else {
		require.ErrorIs(t, physicalSocketDeadlineSupported(fds[0]), os.ErrNoDeadline)
		output := physicalSocketOutput{fd: fds[0], deadline: time.Now().Add(time.Millisecond)}
		n, err := output.Write([]byte("record\n"))
		require.ErrorIs(t, err, os.ErrNoDeadline)
		assert.Zero(t, n)
	}
	after, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFL, 0)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
