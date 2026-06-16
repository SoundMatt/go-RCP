//go:build !linux

package tsn

import (
	rcpudp "github.com/SoundMatt/go-RCP/udp"
)

// setSocketPriority is a no-op on non-Linux platforms.
func setSocketPriority(_ *rcpudp.Controller, _ uint8) {}
