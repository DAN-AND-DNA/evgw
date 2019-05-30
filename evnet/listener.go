package evnet

import (
	"net"
	"os"
	"syscall"
)

type Listener struct {
	m_stLn        net.Listener
	m_stPc        net.PacketConn
	m_pstFile     *os.File
	m_iFd         int
	m_strProtocol string
	m_strAddr     string
}

func (this *Listener) doClose() {

	if this.m_iFd != 0 {
		syscall.Close(this.m_iFd)
	}

	if this.m_pstFile != nil {
		this.m_pstFile.Close()
	}

	if this.m_stLn != nil {
		this.m_stLn.Close()
	}

	if this.m_stPc != nil {
		this.m_stPc.Close()
	}
}
