package evnet

import (
	"strings"
)

// protocl: tcp udp kcp
// address: ip:port
func parseAddr(addr string) (ok bool, protocol string, address string) {
	if !strings.Contains(addr, "://") {
		return false, "", ""
	}

	rawAddr := strings.Split(addr, "://")
	if len(rawAddr) != 2 {
		return false, "", ""
	}

	protocol = rawAddr[0]
	address = rawAddr[1]

	return true, protocol, address
}
