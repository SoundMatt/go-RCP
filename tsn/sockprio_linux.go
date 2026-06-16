//go:build linux

package tsn

import (
	"syscall"

	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

// setSocketPriority sets SO_PRIORITY on the UDP socket so the Linux traffic
// shaper routes the datagram into the correct egress queue for TSN scheduling.
// pcp is the IEEE 802.1p PCP value (0–7); Linux maps it to the SO_PRIORITY
// integer used by the traffic control subsystem.
func setSocketPriority(c *rcpudp.Controller, pcp uint8) {
	rc, err := c.RawConn()
	if err != nil {
		return
	}
	_ = rc.Control(func(fd uintptr) {
		// SO_PRIORITY accepts 0–6 on Linux (7 is reserved for root).
		// Clamp to 6 to avoid EPERM on non-root processes.
		prio := int(pcp)
		if prio > 6 {
			prio = 6
		}
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_PRIORITY, prio) //nolint:errcheck
	})
}
