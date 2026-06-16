package safety_test

//fusa:test REQ-SAFETY-001

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

const (
	latencyDurationEnv = "RCP_LATENCY_DURATION"
	maxSendLatency     = 5 * time.Millisecond // watchdog half-period at 100 Hz
	gcChunkSize        = 64 << 10             // 64 KiB per allocation
	gcIntervalNs       = 977_000              // ~64 MiB/s
	watchdogHz         = 100
	reportPath         = "COMMAND_LATENCY.md"
)

// TestCommandLatencyProfile measures Send and Publish→Subscribe latency under
// sustained GC pressure and writes COMMAND_LATENCY.md as FuSa audit evidence.
// Set RCP_LATENCY_DURATION=30s (or any duration) to enable.
func TestCommandLatencyProfile(t *testing.T) {
	durStr := os.Getenv(latencyDurationEnv)
	if durStr == "" {
		t.Skipf("set %s=<duration> to run the command latency profile (e.g. %s=30s)", latencyDurationEnv, latencyDurationEnv)
	}
	dur, err := time.ParseDuration(durStr)
	if err != nil || dur <= 0 {
		t.Fatalf("invalid %s=%q: %v", latencyDurationEnv, durStr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	zones := []rcp.Zone{
		rcp.ZoneFrontLeft,
		rcp.ZoneFrontRight,
		rcp.ZoneRearLeft,
		rcp.ZoneRearRight,
		rcp.ZoneCentral,
	}

	// -- Set up controllers and subscribers -----------------------------------
	controllers := make([]*mock.Controller, len(zones))
	for i, z := range zones {
		controllers[i] = mock.NewController(z, nil)
		defer controllers[i].Close()
	}

	type subEntry struct {
		ch   <-chan *rcp.Status
		zone rcp.Zone
	}
	subs := make([]subEntry, len(zones))
	for i, ctrl := range controllers {
		ch, err := ctrl.Subscribe(ctx)
		if err != nil {
			t.Fatalf("subscribe zone %s: %v", zones[i], err)
		}
		subs[i] = subEntry{ch: ch, zone: zones[i]}
	}

	// -- Latency accumulators -------------------------------------------------
	var (
		sendSamples    []int64 // nanoseconds
		deliverSamples []int64
	)

	// -- GC pressure goroutine ------------------------------------------------
	go func() {
		ticker := time.NewTicker(time.Duration(gcIntervalNs))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = make([]byte, gcChunkSize)
			}
		}
	}()

	// -- Subscriber drain goroutines ------------------------------------------
	for _, se := range subs {
		go func() {
			for st := range se.ch {
				if len(st.Payload) < 8 {
					continue
				}
				sentNs := int64(binary.BigEndian.Uint64(st.Payload[:8]))
				latNs := time.Now().UnixNano() - sentNs
				if latNs > 0 {
					deliverSamples = append(deliverSamples, latNs)
				}
			}
		}()
	}

	// -- Capture GC state before workload -------------------------------------
	var msBefore, msAfter runtime.MemStats
	runtime.ReadMemStats(&msBefore)

	// -- Workload: Send + Publish at watchdog rate ----------------------------
	watchdogTicker := time.NewTicker(time.Second / watchdogHz)
	defer watchdogTicker.Stop()

	payload := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			goto done
		case <-watchdogTicker.C:
			for i, ctrl := range controllers {
				// Measure Send latency.
				t0 := time.Now()
				_, _ = ctrl.Send(context.Background(), &rcp.Command{
					Zone: zones[i],
					Type: rcp.CmdWatchdog,
					Priority: rcp.PriorityCritical,
				})
				sendSamples = append(sendSamples, time.Since(t0).Nanoseconds())

				// Publish with timestamp payload for delivery latency.
				binary.BigEndian.PutUint64(payload, uint64(time.Now().UnixNano()))
				ctrl.Publish(payload)
			}
		}
	}

done:
	runtime.ReadMemStats(&msAfter)

	// -- Compute percentiles --------------------------------------------------
	p50s, p99s, p999s, maxS := percentiles(sendSamples)
	p50d, p99d, p999d, maxD := percentiles(deliverSamples)

	// -- GC STW stats ---------------------------------------------------------
	var maxPauseNs uint64
	var totalPauseNs uint64
	if msAfter.NumGC > msBefore.NumGC {
		for i := uint32(0); i < msAfter.NumGC-msBefore.NumGC && i < 256; i++ {
			idx := (msAfter.NumGC - 1 - i) % 256
			p := msAfter.PauseNs[idx]
			totalPauseNs += p
			if p > maxPauseNs {
				maxPauseNs = p
			}
		}
	}

	// -- Assert latency bound -------------------------------------------------
	maxSendDur := time.Duration(maxS)
	if maxSendDur > maxSendLatency {
		t.Errorf("REQ-SAFETY-001: Max Send latency %v exceeds watchdog half-period %v",
			maxSendDur, maxSendLatency)
	}

	// -- Write evidence report ------------------------------------------------
	report := buildReport(dur, len(sendSamples), len(deliverSamples),
		p50s, p99s, p999s, maxS,
		p50d, p99d, p999d, maxD,
		maxPauseNs, totalPauseNs, msAfter.NumGC-msBefore.NumGC)

	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Logf("warning: could not write %s: %v", reportPath, err)
	} else {
		t.Logf("evidence written to %s", reportPath)
	}
}

func percentiles(ns []int64) (p50, p99, p999, max int64) {
	if len(ns) == 0 {
		return 0, 0, 0, 0
	}
	sorted := make([]int64, len(ns))
	copy(sorted, ns)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p50 = sorted[idx(len(sorted), 0.50)]
	p99 = sorted[idx(len(sorted), 0.99)]
	p999 = sorted[idx(len(sorted), 0.999)]
	max = sorted[len(sorted)-1]
	return
}

func idx(n int, pct float64) int {
	i := int(math.Ceil(pct*float64(n))) - 1
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	return i
}

func buildReport(dur time.Duration, nSend, nDeliver int,
	p50s, p99s, p999s, maxS, p50d, p99d, p999d, maxD int64,
	maxPauseNs, totalPauseNs uint64, gcRuns uint32) string {

	threshold := maxSendLatency.Microseconds()
	maxSus := time.Duration(maxS).Microseconds()
	pass := "PASS"
	if maxSus >= threshold {
		pass = "FAIL"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Command Latency Safety Evidence\n\n")
	fmt.Fprintf(&b, "Generated by `TestCommandLatencyProfile` — go-RCP v0.3.0\n\n")
	fmt.Fprintf(&b, "**Workload duration:** %v  \n", dur)
	fmt.Fprintf(&b, "**GC pressure:** ~64 MiB/s (64 KiB allocations at ~1 ms interval)  \n")
	fmt.Fprintf(&b, "**Watchdog rate:** %d Hz (one `CmdWatchdog` Send per zone per tick)  \n\n", watchdogHz)
	fmt.Fprintf(&b, "---\n\n")

	fmt.Fprintf(&b, "## GSN Safety Argument\n\n")

	fmt.Fprintf(&b, "**Claim (C-1):** go-RCP command delivery on the mock transport meets the "+
		"%d µs watchdog half-period latency budget under sustained GC pressure.\n\n", threshold)

	fmt.Fprintf(&b, "**Goal (G-1):** Demonstrate that Max(Send latency) < %d µs over a %v "+
		"workload with ~64 MiB/s GC allocation pressure, with ≥ 10 000 samples.\n\n", threshold, dur)

	fmt.Fprintf(&b, "**Strategy (S-1):** Empirical measurement using the in-process mock "+
		"controller under realistic GC load. Latency sampled for every `Send` call. "+
		"Percentiles computed over the full sample set.\n\n")

	fmt.Fprintf(&b, "**Evidence (E-1):**\n\n")
	fmt.Fprintf(&b, "### Send latency (CmdWatchdog, %d samples)\n\n", nSend)
	fmt.Fprintf(&b, "| Metric | Measured | Threshold | Result |\n")
	fmt.Fprintf(&b, "|--------|----------|-----------|--------|\n")
	fmt.Fprintf(&b, "| P50    | %d µs   | —         | —      |\n", time.Duration(p50s).Microseconds())
	fmt.Fprintf(&b, "| P99    | %d µs   | —         | —      |\n", time.Duration(p99s).Microseconds())
	fmt.Fprintf(&b, "| P99.9  | %d µs   | —         | —      |\n", time.Duration(p999s).Microseconds())
	fmt.Fprintf(&b, "| Max    | %d µs   | < %d µs  | **%s** |\n\n", maxSus, threshold, pass)

	fmt.Fprintf(&b, "### Publish→Subscribe delivery latency (%d samples)\n\n", nDeliver)
	fmt.Fprintf(&b, "| Metric | Measured |\n")
	fmt.Fprintf(&b, "|--------|----------|\n")
	fmt.Fprintf(&b, "| P50    | %d µs   |\n", time.Duration(p50d).Microseconds())
	fmt.Fprintf(&b, "| P99    | %d µs   |\n", time.Duration(p99d).Microseconds())
	fmt.Fprintf(&b, "| P99.9  | %d µs   |\n", time.Duration(p999d).Microseconds())
	fmt.Fprintf(&b, "| Max    | %d µs   |\n\n", time.Duration(maxD).Microseconds())

	fmt.Fprintf(&b, "### GC stop-the-world pauses\n\n")
	fmt.Fprintf(&b, "| Metric            | Value      |\n")
	fmt.Fprintf(&b, "|-------------------|------------|\n")
	fmt.Fprintf(&b, "| GC cycles         | %d         |\n", gcRuns)
	fmt.Fprintf(&b, "| Max STW pause     | %d µs      |\n", time.Duration(maxPauseNs).Microseconds())
	fmt.Fprintf(&b, "| Total STW time    | %d µs      |\n\n", time.Duration(totalPauseNs).Microseconds())

	fmt.Fprintf(&b, "**Assumptions (A-1):**\n\n")
	fmt.Fprintf(&b, "- Transport latency (UDP/TLS) is not characterised here; only the library layer overhead is measured.\n")
	fmt.Fprintf(&b, "- GOMAXPROCS is at the OS default; CPU isolation (cgroups + IRQ affinity) can further reduce jitter.\n")
	fmt.Fprintf(&b, "- The mock controller is single-process in-memory; network latency is absent by design.\n\n")

	fmt.Fprintf(&b, "**Residual risk (R-1):**\n\n")
	fmt.Fprintf(&b, "- Real zone controllers operate over a physical Ethernet network, adding non-deterministic latency not measured here.\n")
	fmt.Fprintf(&b, "- GC STW pauses of > 1 ms are possible under extreme allocation rates. The TSN transport (v0.10.0) will use hardware timestamping to compensate.\n")
	fmt.Fprintf(&b, "- This evidence covers the mock transport only. Each transport implementation requires its own latency characterisation before integration.\n")
	return b.String()
}
