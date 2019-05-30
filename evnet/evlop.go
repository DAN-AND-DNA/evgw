package evnet

import (
	"errors"
	"syscall"
)

type Evlop struct {
	m_iEpollFd int
	m_stIn     []byte
	m_stConns  []realConn
}

func newEvlop(numConns int, numInBuf int) (error, *Evlop) {
	epollFd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		if epollFd != 0 {
			syscall.Close(epollFd)
		}
		return err, nil
	}

	return nil, &Evlop{
		m_iEpollFd: epollFd,
		m_stConns:  make([]realConn, numConns),
		m_stIn:     make([]byte, numInBuf+1),
	}
}

func (this *Evlop) doClose() error {
	return syscall.Close(this.m_iEpollFd)
}

func (this *Evlop) wait(events []syscall.EpollEvent) (error, int) {
	if len(events) == 0 {
		return errors.New("events is empty"), 0
	}

	n, err := syscall.EpollWait(this.m_iEpollFd, events, -1)
	if err != nil && err != syscall.EINTR {
		return err, 0
	}

	return nil, n
}

func (this *Evlop) getConn(fd int) (error, *realConn) {
	if fd > len(this.m_stConns) {
		return errors.New("overflow"), nil
	}

	return nil, &this.m_stConns[fd]
}

func (this *Evlop) addConn(conn realConn) bool {
	if conn.m_iFd <= 0 {
		return false
	}
	this.m_stConns[conn.m_iFd] = conn
	return true
}

func (this *Evlop) enableAll(pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("unaccepted conn")
	}

	op := syscall.EPOLL_CTL_ADD
	fd := pstConn.m_iFd
	if pstConn.m_iFiredEvents == 0 {
	} else {
		op = syscall.EPOLL_CTL_MOD
	}
	pstConn.m_iFiredEvents |= syscall.EPOLLIN
	pstConn.m_iFiredEvents |= syscall.EPOLLOUT
	return syscall.EpollCtl(this.m_iEpollFd, op, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: (uint32)(pstConn.m_iFiredEvents)})
}

func (this *Evlop) enableRead(pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("unaccepted conn")
	}

	op := syscall.EPOLL_CTL_ADD
	fd := pstConn.m_iFd
	if pstConn.m_iFiredEvents == 0 {
	} else {
		op = syscall.EPOLL_CTL_MOD
	}
	pstConn.m_iFiredEvents |= syscall.EPOLLIN
	return syscall.EpollCtl(this.m_iEpollFd, op, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: (uint32)(pstConn.m_iFiredEvents)})

}

func (this *Evlop) enableWrite(pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("unaccepted conn")
	}

	op := syscall.EPOLL_CTL_ADD
	fd := pstConn.m_iFd
	if pstConn.m_iFiredEvents == 0 {
	} else {
		op = syscall.EPOLL_CTL_MOD
	}
	pstConn.m_iFiredEvents |= syscall.EPOLLOUT
	return syscall.EpollCtl(this.m_iEpollFd, op, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: (uint32)(pstConn.m_iFiredEvents)})

}

func (this *Evlop) disableAll(pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("unaccepted conn")
	}

	fd := pstConn.m_iFd
	if pstConn.m_iFiredEvents == 0 {
		return nil
	} else {
		op := syscall.EPOLL_CTL_DEL
		pstConn.m_iFiredEvents = 0
		return syscall.EpollCtl(this.m_iEpollFd, op, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: syscall.EPOLLOUT})
	}
}

func (this *Evlop) disableRead(pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("unaccepted conn")
	}

	fd := pstConn.m_iFd
	if pstConn.m_iFiredEvents == 0 {
		return nil
	} else {
		op := syscall.EPOLL_CTL_MOD
		pstConn.m_iFiredEvents &= ^syscall.EPOLLIN
		if pstConn.m_iFiredEvents == 0 {
			op = syscall.EPOLL_CTL_DEL
		}
		return syscall.EpollCtl(this.m_iEpollFd, op, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: (uint32)(pstConn.m_iFiredEvents)})
	}
}

func (this *Evlop) disableWrite(pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("unaccepted conn")
	}

	fd := pstConn.m_iFd
	if pstConn.m_iFiredEvents == 0 {
		return nil
	} else {
		op := syscall.EPOLL_CTL_MOD
		pstConn.m_iFiredEvents &= ^syscall.EPOLLOUT
		if pstConn.m_iFiredEvents == 0 {
			op = syscall.EPOLL_CTL_DEL
		}
		return syscall.EpollCtl(this.m_iEpollFd, op, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: (uint32)(pstConn.m_iFiredEvents)})
	}

}
