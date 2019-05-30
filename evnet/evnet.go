package evnet

import (
	"errors"
	"net"
	"runtime"
	"syscall"
)

type Evs struct {
	m_iNumLoops  int
	m_iNumOutBuf int
	m_iNumInBuf  int
	m_iNumConns  int
	Opened       func(c Conn, out []byte) (action Action)
	Close        func(c Conn) (action Action)
	Data         func(c Conn, in []byte, out []byte) (action Action)
}

type Evnet struct {
	m_stEvents     Evs
	m_stListeners  []*Listener
	m_stEventloops []*Evlop
}

func (this *Evnet) makeListener(addrs ...string) error {
	lns := this.m_stListeners
	for _, addr := range addrs {
		if ok, protocol, address := parseAddr(addr); ok {
			ln := Listener{
				m_strProtocol: protocol,
				m_strAddr:     address,
			}

			var err error
			if protocol == "tcp" {
				ln.m_stLn, err = net.Listen("tcp", address)
				if err != nil {
					return err
				}

				switch realType := ln.m_stLn.(type) {
				case nil:
					return errors.New("get real type failed")
				case *net.TCPListener:
					if ln.m_pstFile, err = realType.File(); err != nil {
						return err
					}
				}
			} else if protocol == "udp" {
				ln.m_stPc, err = net.ListenPacket("udp", address)

				if err != nil {
					return err
				}
				switch realType := ln.m_stPc.(type) {
				case nil:
					return errors.New("get real type failed")
				case *net.UDPConn:
					if ln.m_pstFile, err = realType.File(); err != nil {
						return nil
					}
				}
			} else if protocol == "kcp" {
				ln.m_stPc, err = net.ListenPacket("udp", address)

				if err != nil {
					return err
				}
				switch realType := ln.m_stPc.(type) {
				case nil:
					return errors.New("get real type failed")
				case *net.UDPConn:
					if ln.m_pstFile, err = realType.File(); err != nil {
						return nil
					}
				}
			}

			ln.m_iFd = (int)(ln.m_pstFile.Fd())
			syscall.SetNonblock(ln.m_iFd, true)
			lns = append(lns, &ln)
		}
	}
	return nil

}

func (this *Evnet) closeListener() {
	for _, ln := range this.m_stListeners {
		ln.doClose()
	}
}

func (this *Evnet) makeServer(numLoops int, numConns int, numInBuf int) error {
	if numLoops <= 0 {
		if numLoops == 0 {
			numLoops = 1
		} else {
			numLoops = runtime.NumCPU()
		}
	}

	for i := 0; i < numLoops; i++ {
		err, pstEventloop := newEvlop(numConns, numInBuf)
		if err != nil {
			return err
		}
		this.m_stEventloops = append(this.m_stEventloops, pstEventloop)
		go this.Loop(pstEventloop)
	}
	return nil
}

func (this *Evnet) closeEvlop() {
	if this == nil {
		return
	}
	for _, lp := range this.m_stEventloops {
		lp.doClose()
	}
}

func (this *Evnet) loopAccept(listenFd int, pstEvlop *Evlop, pstConn *realConn) error {
	for _, listener := range this.m_stListeners {
		if listener.m_iFd == listenFd {
			clientFd, _, err := syscall.Accept4(listenFd, syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK)
			if err != nil {
				if err == syscall.EAGAIN {
					return nil
				}
				return err
			}

			if listener.m_stPc != nil {
				return this.loopUDPRead(pstEvlop)
			}

			pstConn.m_iFd = clientFd
			pstConn.m_stOut = make([]byte, this.m_stEvents.m_iNumOutBuf)
			pstConn.m_stCurr = pstConn.m_stOut
			pstConn.m_pstLoop = pstEvlop
			pstConn.m_iFiredEvents = 0
			pstConn.m_stStatus = Accepted
			pstEvlop.enableAll(pstConn)
		}
	}
	return nil
}

func (this *Evnet) loopOpen(pstEvlop *Evlop, pstConn *realConn) error {
	if this.m_stEvents.Opened != nil {
		action := this.m_stEvents.Opened(pstConn, pstConn.m_stCurr)
		_ = action
	}

	if len(pstConn.m_stOut)-len(pstConn.m_stCurr) == 0 && pstConn.m_iFiredEvents == 0 {
		pstEvlop.disableWrite(pstConn)

	}
	pstConn.m_stStatus = Opened
	return nil
}

func (this *Evnet) loopCloseConn(pstEvlop *Evlop, pstConn *realConn) error {
	if pstConn.m_stStatus != Accepted {
		return errors.New("close unaccepted conn")
	}

	if this.m_stEvents.Close != nil {
		action := this.m_stEvents.Close(pstConn)
		_ = action
	}
	pstEvlop.m_stConns[pstConn.m_iFd] = realConn{}
	pstEvlop.disableAll(pstConn)
	syscall.Close(pstConn.m_iFd)
	return nil

}

func (this *Evnet) loopUDPRead(pstEvlop *Evlop) error {
	if this.m_stEvents.Data != nil {
	}

	return nil

}
func (this *Evnet) loopRead(pstEvlop *Evlop, pstConn *realConn) error {
	if this.m_stEvents.Data != nil {
		n, err := syscall.Read(pstConn.m_iFd, pstEvlop.m_stIn)
		if n == 0 || err != nil {
			if err == syscall.EAGAIN {
				return nil
			}
			//eof
			return this.loopCloseConn(pstEvlop, pstConn)
		}

		if n > this.m_stEvents.m_iNumInBuf {
			// reach limit
			return this.loopCloseConn(pstEvlop, pstConn)
		}

		in := pstEvlop.m_stIn[:n]
		action := this.m_stEvents.Data(pstConn, in, pstConn.m_stCurr)
		_ = action

		if len(pstConn.m_stOut)-len(pstConn.m_stCurr) != 0 {
			pstEvlop.enableWrite(pstConn)
		}
	}
	return nil
}

func (this *Evnet) loopWrite(pstEvlop *Evlop, pstConn *realConn) error {
	m := len(pstConn.m_stOut) - len(pstConn.m_stCurr)
	out := pstConn.m_stOut[:m]
	n, err := syscall.Write(pstConn.m_iFd, out)

	if err != nil {
		if err == syscall.EAGAIN {
			return nil
		}
		return this.loopCloseConn(pstEvlop, pstConn)
	}
	pstConn.m_stCurr = pstConn.m_stOut[m-n:]

	if m == n {
		pstEvlop.disableWrite(pstConn)
	}

	return nil
}

func (this *Evnet) Loop(pstEvlop *Evlop) {
	if pstEvlop == nil || this == nil {
		return
	}
	events := make([]syscall.EpollEvent, 64)
	for {
		err, numFired := pstEvlop.wait(events)
		if err != nil {
			return
		}

		for i := 0; i < numFired; i++ {
			firedFd := (int)(events[i].Fd)
			err, pstConn := pstEvlop.getConn(firedFd)
			if err != nil {
				return
			}
			switch {
			case pstConn.m_stStatus == Unkown:
				if err := this.loopAccept(firedFd, pstEvlop, pstConn); err != nil {
					return
				}
			case pstConn.m_stStatus == Accepted:
				if err := this.loopOpen(pstEvlop, pstConn); err != nil {
					return
				}
			case this.m_stEvents.m_iNumOutBuf-len(pstConn.m_stCurr) > 0:
				if err := this.loopWrite(pstEvlop, pstConn); err != nil {
					return
				}
			default:
				if err := this.loopRead(pstEvlop, pstConn); err != nil {
					return
				}
			}
		}
	}
}

func (this *Evnet) Serve(evs Evs, addrs ...string) error {
	err := this.makeListener(addrs...)
	if err != nil {
		panic(err)
	}
	defer this.closeListener()
	err = this.makeServer(evs.m_iNumLoops, evs.m_iNumInBuf, evs.m_iNumConns)
	if err != nil {
		panic(err)
	}
	defer this.closeEvlop()
	return nil
}
