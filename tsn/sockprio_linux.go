//go:build linux

package tsn

import (
	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

// setSocketPriority is a best-effort hook for Linux SO_PRIORITY.
// Without direct access to the fd, we log the intent but do not set it.
// Full TSN SO_PRIORITY support requires the UDP package to expose the raw fd.
func setSocketPriority(_ *rcpudp.Controller, _ uint8) {
	// Best-effort: no-op until udp.Controller exposes RawConn.
}
