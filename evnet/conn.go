package evnet

type Status int

const (
	Unkown Status = iota
	Accepted
	Opened
)

type Action int

const (
	None Action = iota
	Close
)

type Conn interface {
	Context() interface{}
	SetContext(interface{})
}

type realConn struct {
	m_iFd    int
	m_stOut  []byte
	m_stCurr []byte
	//	m_stIn         []byte
	m_pstLoop      *Evlop
	m_iFiredEvents int
	m_stStatus     Status
	m_stCtx        interface{}
}

func (this *realConn) Context() interface{} {
	return this.m_stCtx
}

func (this *realConn) SetContext(ctx interface{}) {
	this.m_stCtx = ctx
}
