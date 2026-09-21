package service

import (
	"errors"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

func openPhysicalOutput() (physicalOutput, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(2, &stat); err != nil {
		return nil, err
	}
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFSOCK:
		if err := physicalSocketDeadlineSupported(2); err != nil {
			return nil, err
		}
		fd, err := unix.Dup(2)
		if err != nil {
			return nil, err
		}
		unix.CloseOnExec(fd)
		return &physicalSocketOutput{fd: fd}, nil
	case unix.S_IFIFO:
		if runtime.GOOS == "linux" {
			output, err := os.OpenFile("/proc/self/fd/2", os.O_WRONLY|unix.O_NONBLOCK, 0)
			if err != nil {
				return nil, err
			}
			if err := output.SetWriteDeadline(time.Now().Add(physicalWriteTimeout)); err != nil {
				_ = output.Close()
				return nil, err
			}
			return output, nil
		}
	}
	return nil, os.ErrNoDeadline
}

type physicalSocketOutput struct {
	fd       int
	deadline time.Time
}

func (output *physicalSocketOutput) SetWriteDeadline(deadline time.Time) error {
	output.deadline = deadline
	return nil
}

func (output *physicalSocketOutput) Close() error { return unix.Close(output.fd) }

func physicalSocketDeadlineSupported(fd int) error {
	if runtime.GOOS == "linux" {
		return nil
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return err
	}
	if flags&unix.O_NONBLOCK == 0 {
		return os.ErrNoDeadline
	}
	return nil
}

func (output *physicalSocketOutput) Write(value []byte) (int, error) {
	if err := physicalSocketDeadlineSupported(output.fd); err != nil {
		return 0, err
	}
	written := 0
	for written < len(value) {
		remaining := time.Until(output.deadline)
		if remaining <= 0 {
			return written, os.ErrDeadlineExceeded
		}
		n, err := unix.SendmsgN(output.fd, value[written:], nil, nil, unix.MSG_DONTWAIT|unix.MSG_NOSIGNAL)
		if n > 0 {
			written += n
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			milliseconds := int((remaining + time.Millisecond - 1) / time.Millisecond)
			_, err = unix.Poll([]unix.PollFd{{Fd: int32(output.fd), Events: unix.POLLOUT}}, milliseconds)
			if err == nil || errors.Is(err, unix.EINTR) {
				continue
			}
		}
		if err != nil {
			return written, err
		}
	}
	return written, nil
}
