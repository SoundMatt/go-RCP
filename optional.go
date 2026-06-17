//fusa:req REQ-OPT-001
//fusa:req REQ-OPT-002
//fusa:req REQ-OPT-003
//fusa:req REQ-OPT-004
//fusa:req REQ-OPT-005
//fusa:req REQ-OPT-006

package rcp

import (
	"context"
	"strconv"
	"time"

	relay "github.com/SoundMatt/RELAY"
)

// Compile-time assertions that the adapter satisfies the optional RELAY
// interfaces (spec §9). These are declared under "optional_interfaces" in the
// CLI capabilities document.
var (
	_ relay.HealthProvider  = (*rcpAdapter)(nil)
	_ relay.MetricsProvider = (*rcpAdapter)(nil)
	_ relay.Drainer         = (*rcpAdapter)(nil)
)

// Health reports the adapter's coarse health (spec §9).
// A closed adapter is HealthDown; one that has recorded errors but is still
// open is HealthDegraded; otherwise HealthOK.
//
//fusa:req REQ-OPT-001
//fusa:req REQ-OPT-002
func (a *rcpAdapter) Health() relay.Health {
	if a.closed.Load() {
		return relay.Health{Status: relay.HealthDown, Details: "adapter closed"}
	}
	if n := a.errorCount.Load(); n > 0 {
		return relay.Health{
			Status:  relay.HealthDegraded,
			Details: strconv.FormatUint(n, 10) + " errors recorded",
		}
	}
	return relay.Health{Status: relay.HealthOK}
}

// Metrics returns a snapshot of the adapter's runtime counters (spec §9).
//
//fusa:req REQ-OPT-003
//fusa:req REQ-OPT-004
func (a *rcpAdapter) Metrics() relay.Metrics {
	return relay.Metrics{
		WriteCount:     a.writeCount.Load(),
		DeliverCount:   a.deliverCount.Load(),
		DropCount:      a.dropCount.Load(),
		BytesWritten:   a.bytesWritten.Load(),
		BytesDelivered: a.bytesDelivered.Load(),
		ErrorCount:     a.errorCount.Load(),
	}
}

// CloseWithDrain waits for in-flight Send/Call dispatches to complete before
// closing the underlying Controller (spec §9). If ctx expires first it closes
// immediately and returns ctx.Err().
//
//fusa:req REQ-OPT-005
//fusa:req REQ-OPT-006
func (a *rcpAdapter) CloseWithDrain(ctx context.Context) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for a.inFlight.Load() > 0 {
		select {
		case <-ctx.Done():
			_ = a.Close()
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return a.Close()
}
