package mdns

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// Browser discovers zone controllers on the mDNS bus.
// The caller owns the Transport; Browser borrows it and never closes it.
type Browser struct {
	transport Transport
	closed    atomic.Bool
}

// Handler is called once for each discovered zone controller.
// zone is the numeric Zone value; addr is "ip:port".
type Handler func(zone uint8, addr string)

// NewBrowser creates a Browser using the given transport.
// Pass nil to open real multicast UDP.
func NewBrowser(transport Transport) (*Browser, error) {
	if transport == nil {
		var err error
		transport, err = NewMulticastTransport(nil)
		if err != nil {
			return nil, err
		}
	}
	return &Browser{transport: transport}, nil
}

// Query sends one PTR query and reads responses until ctx is done, calling h for each discovered service.
// Query returns when ctx is cancelled; the Transport is NOT closed.
func (b *Browser) Query(ctx context.Context, h Handler) error {
	if b.closed.Load() {
		return fmt.Errorf("mdns: browser closed")
	}
	if err := b.transport.WritePacket(buildPTRQuery()); err != nil {
		return fmt.Errorf("mdns: send query: %w", err)
	}

	// Cancel the blocking ReadPacket when ctx is done by setting a past deadline.
	go func() {
		<-ctx.Done()
		_ = b.transport.SetReadDeadline(time.Now())
	}()

	buf := make([]byte, 65536)
	for {
		n, err := b.transport.ReadPacket(buf)
		if err != nil {
			// If ctx is done, this is expected (deadline exceeded).
			select {
			case <-ctx.Done():
				_ = b.transport.SetReadDeadline(time.Time{}) // reset for reuse
				return nil
			default:
				return err
			}
		}
		msg := make([]byte, n)
		copy(msg, buf[:n])

		rrs, err := parseRRs(msg)
		if err != nil || len(rrs) == 0 {
			continue
		}
		hdr, err := decodeHeader(msg)
		if err != nil {
			continue
		}
		if hdr.flags&0x8000 == 0 {
			continue // ignore queries
		}
		for _, si := range extractServices(rrs, msg) {
			h(si.Zone, si.Addr)
		}
	}
}

// Close marks the Browser as closed. It does NOT close the underlying Transport.
func (b *Browser) Close() error {
	b.closed.Store(true)
	return nil
}
