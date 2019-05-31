package evnet

import (
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
)

type Evs struct {
	m_iNumLoops  int
	m_iNumOutBuf int
	m_iNumInBuf  int
	m_iNumConns  int
	Serving      func() (action Action)
	Opened       func(c Conn, out []byte) (action Action)
	Closed       func(c Conn, err error) (action Action)
	Data         func(c Conn, in []byte, out []byte) (action Action)
}

func NewConfig() Evs {
	return Evs{
		m_iNumLoops:  4,
		m_iNumOutBuf: 65536 * 4,
		m_iNumInBuf:  65536 * 2,
		m_iNumConns:  20000,
	}
}

type Evnet struct {
	m_stEvents     Evs
	m_stEventloops []*Evlop
	m_stListeners  []*Listener
	m_stWg         sync.WaitGroup
}

func NewEvnet() *Evnet {
	return &Evnet{}
}

func (this *Evnet) makeListener(addrs ...string) error {
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
				log.Println("tcp")

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

				log.Println("udp")
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
			} else {
				return errors.New("bad protocol")
			}

			ln.m_iFd = (int)(ln.m_pstFile.Fd())
			syscall.SetNonblock(ln.m_iFd, true)
			this.m_stListeners = append(this.m_stListeners, &ln)
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

		for _, listener := range this.m_stListeners {
			epollfd := pstEventloop.m_iEpollFd
			fd := listener.m_iFd
			err := syscall.EpollCtl(epollfd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Fd: (int32)(fd), Events: (uint32)(syscall.EPOLLIN)})

			if err != nil {
				return err
			}
		}

		this.m_stEventloops = append(this.m_stEventloops, pstEventloop)
		this.m_stWg.Add(1)
		log.Println("+1")
		go this.loop(pstEventloop)
	}
	return nil
}

func (this *Evnet) closeServer() {
	if this == nil {
		return
	}

	this.m_stWg.Wait()

	for _, lp := range this.m_stEventloops {
		for k := range lp.m_stConns {
			pstConn := &lp.m_stConns[k]
			this.loopCloseConn(lp, pstConn, nil)
		}
		lp.doClose()
	}
}

func (this *Evnet) loopAccept(listenFd int, pstEvlop *Evlop) error {
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

			if clientFd >= this.m_stEvents.m_iNumConns {
				return errors.New("reach conns limit")
			}

			pstConn := &pstEvlop.m_stConns[clientFd]
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

func (this *Evnet) loopOpened(pstEvlop *Evlop, pstConn *realConn) error {
	if this.m_stEvents.Opened != nil {
		action := this.m_stEvents.Opened(pstConn, pstConn.m_stCurr)
		_ = action
	}

	if len(pstConn.m_stOut)-len(pstConn.m_stCurr) == 0 {
		log.Println("disable write")
		pstEvlop.disableWrite(pstConn)

	}
	pstConn.m_stStatus = Opened
	return nil
}

func (this *Evnet) loopCloseConn(pstEvlop *Evlop, pstConn *realConn, err error) error {
	if pstConn.m_stStatus != Opened {
		return errors.New("close unclosed conn")
	}

	if this.m_stEvents.Closed != nil {
		action := this.m_stEvents.Closed(pstConn, err)
		_ = action
	}
	log.Printf("disable all")
	pstEvlop.disableAll(pstConn)
	syscall.Close(pstConn.m_iFd)
	pstEvlop.m_stConns[pstConn.m_iFd] = realConn{}
	return nil
}

func (this *Evnet) loopUDPRead(pstEvlop *Evlop) error {
	if this.m_stEvents.Data != nil {
	}

	return nil

}
func (this *Evnet) loopRead(pstEvlop *Evlop, pstConn *realConn) error {
	n, err := syscall.Read(pstConn.m_iFd, pstEvlop.m_stIn)
	if n == 0 || err != nil {
		if err == syscall.EAGAIN {
			return nil
		}
		//eof
		return this.loopCloseConn(pstEvlop, pstConn, err)
	}

	if n > this.m_stEvents.m_iNumInBuf {
		// reach limit
		return this.loopCloseConn(pstEvlop, pstConn, errors.New("reach in limit"))
	}

	if this.m_stEvents.Data != nil {
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
		return this.loopCloseConn(pstEvlop, pstConn, err)
	}
	pstConn.m_stCurr = pstConn.m_stOut[m-n:]

	if m == n {
		pstEvlop.disableWrite(pstConn)
	}

	return nil
}

func (this *Evnet) loop(pstEvlop *Evlop) {
	if pstEvlop == nil || this == nil {
		return
	}

	defer func() {
		this.m_stWg.Done()
		log.Println("-1")
	}()

	if this.m_stEvents.Serving != nil {
		action := this.m_stEvents.Serving()
		_ = action
	}

	events := make([]syscall.EpollEvent, 64)
	for {
		err, numFired := pstEvlop.wait(events)
		if err != nil {
			return
		}

		for i := 0; i < numFired; i++ {
			firedFd := (int)(events[i].Fd)
			if firedFd == pstEvlop.m_iWakeFd {
				return
			}
			err, pstConn := pstEvlop.getConn(firedFd)
			if err != nil {
				return
			}
			switch {
			case pstConn.m_stStatus == Unkown:
				log.Println("u")
				if err := this.loopAccept(firedFd, pstEvlop); err != nil {
					return
				}
			case pstConn.m_stStatus == Accepted:
				log.Println("a")
				if err := this.loopOpened(pstEvlop, pstConn); err != nil {
					return
				}
			case this.m_stEvents.m_iNumOutBuf-len(pstConn.m_stCurr) > 0:
				log.Println("w")
				if err := this.loopWrite(pstEvlop, pstConn); err != nil {
					return
				}
			default:
				log.Println("r")
				if err := this.loopRead(pstEvlop, pstConn); err != nil {
					return
				}
			}
		}
	}
}

func (this *Evnet) Stop() {
	for _, pstEventloop := range this.m_stEventloops {
		pstEventloop.wake()
	}
}

func (this *Evnet) Serve(evs Evs, addrs ...string) error {
	err := this.makeListener(addrs...)
	if err != nil {
		panic(err)
	}
	defer this.closeListener()
	this.m_stEvents = evs
	err = this.makeServer(evs.m_iNumLoops, evs.m_iNumInBuf, evs.m_iNumConns)
	if err != nil {
		panic(err)
	}
	defer this.closeServer()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("get Ctrl+c , closing server...")
	this.Stop()
	return nil
}
