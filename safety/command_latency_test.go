package safety_test

//fusa:test REQ-SAFETY-001

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/request"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

const (
	latencyDurationEnv = "RCP_LATENCY_DURATION"
	maxSendLatency     = 5 * time.Millisecond // watchdog half-period at 100 Hz
	gcChunkSize        = 64 << 10             // 64 KiB per allocation
	gcIntervalNs       = 977_000              // ~64 MiB/s
	watchdogHz         = 100
	reportPath         = "COMMAND_LATENCY.md"
	endpointCount      = 5 // mirrors the pre-TC18 evidence's 5-zone workload
)

// dispatchHandler adapts a *request.Dispatcher to the udp.Router's
// request.Handler shape (HandleRequest(requester, req) (resp, err)) via
// Dispatcher.Dispatch — the same "wrap, don't edit" pattern e2e.Guard and
// every Phase 14 endpoint type already use, applied here so this test can
// route a live UDP round trip through the actual request-lifecycle state
// machine (ROADMAP.md Milestone 49) rather than calling a Handler directly
// in-process.
type dispatchHandler struct {
	d *request.Dispatcher
}

func (h *dispatchHandler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	return h.d.Dispatch(requester, req, uint64(time.Now().UnixNano()))
}

// echoHandler answers every plain request with a fixed-shape success
// response, echoing the Read/Write flag and correlation fields — this
// test's stand-in for a real endpoint type, chosen because this milestone
// measures the transport+dispatch path's own overhead, not any specific
// endpoint's business logic (the same posture the pre-TC18 evidence's mock
// controller took toward CmdWatchdog).
type echoHandler struct{}

func (echoHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           []byte{0x01},
	}, nil
}

// TestCommandLatencyProfile measures request.Dispatcher/udp.Controller.Write
// round-trip latency under sustained GC pressure and writes
// COMMAND_LATENCY.md as FuSa audit evidence. Set RCP_LATENCY_DURATION=30s
// (or any duration) to enable.
//
// This measures the TC18 request/response path (ROADMAP.md Milestone 58,
// v0.71.0): a real udp.Server/udp.Router pair, each registered address
// backed by a request.Dispatcher (via dispatchHandler above), driven over
// an actual loopback UDP socket by udp.Controller.Write — replacing the
// retired pre-TC18 evidence's rcp.Controller.Send/mock.Controller
// in-process measurement, per this milestone's own re-scoping (see
// safety/doc.go).
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

	// -- Set up the server: one request.Dispatcher per registered address --
	// This workload never addresses regmap.EP0, so the Router's EP0 handler
	// is left nil rather than wiring up an unused *server.Server.
	router := udp.NewRouter(nil, false)
	addrs := make([]avtp.ByteBusID, endpointCount)
	for i := 0; i < endpointCount; i++ {
		addrs[i] = avtp.ByteBusID(i + 1)
		d := request.NewDispatcher(echoHandler{}, addrs[i], request.NewSequencer(), nil)
		if regErr := router.Register(addrs[i], &dispatchHandler{d: d}); regErr != nil {
			t.Fatalf("Register(%d): %v", addrs[i], regErr)
		}
	}
	serverStream := avtp.NewStreamID([6]byte{0x06, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
	srv, err := udp.NewServer(serverStream, "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("udp.NewServer: %v", err)
	}
	defer func() { _ = srv.Close() }()

	// -- Dial one Controller per address, mirroring one-controller-per-zone --
	controllers := make([]*udp.Controller, endpointCount)
	for i := range controllers {
		stream := avtp.NewStreamID([6]byte{0x06, 0x11, 0x22, 0x33, 0x44, byte(i)}, 1)
		ctrl, err := udp.NewController(stream, srv.Addr())
		if err != nil {
			t.Fatalf("udp.NewController[%d]: %v", i, err)
		}
		controllers[i] = ctrl
		defer func() { _ = ctrl.Close() }()
	}

	// -- Latency accumulator --------------------------------------------------
	var sendSamples []int64 // nanoseconds

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

	// -- Capture GC state before workload -------------------------------------
	var msBefore, msAfter runtime.MemStats
	runtime.ReadMemStats(&msBefore)

	// -- Workload: udp.Controller.Write at watchdog rate ----------------------
	watchdogTicker := time.NewTicker(time.Second / watchdogHz)
	defer watchdogTicker.Stop()

	body := []byte{0x01}
	for {
		select {
		case <-ctx.Done():
			goto done
		case <-watchdogTicker.C:
			for i, ctrl := range controllers {
				t0 := time.Now()
				reqCtx, reqCancel := context.WithTimeout(context.Background(), maxSendLatency*4)
				_, _ = ctrl.Write(reqCtx, addrs[i], body)
				reqCancel()
				sendSamples = append(sendSamples, time.Since(t0).Nanoseconds())
			}
		}
	}

done:
	runtime.ReadMemStats(&msAfter)

	// -- Compute percentiles --------------------------------------------------
	p50s, p99s, p999s, maxS := percentiles(sendSamples)

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
	report := buildReport(dur, len(sendSamples), p50s, p99s, p999s, maxS,
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

func buildReport(dur time.Duration, nSend int,
	p50s, p99s, p999s, maxS int64,
	maxPauseNs, totalPauseNs uint64, gcRuns uint32) string {

	threshold := maxSendLatency.Microseconds()
	maxSus := time.Duration(maxS).Microseconds()
	pass := "PASS"
	if maxSus >= threshold {
		pass = "FAIL"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Command Latency Safety Evidence\n\n")
	fmt.Fprintf(&b, "Generated by `TestCommandLatencyProfile` — go-RCP v0.71.0\n\n")
	fmt.Fprintf(&b, "**Workload duration:** %v  \n", dur)
	fmt.Fprintf(&b, "**GC pressure:** ~64 MiB/s (64 KiB allocations at ~1 ms interval)  \n")
	fmt.Fprintf(&b, "**Watchdog rate:** %d Hz (one `udp.Controller.Write` per endpoint per tick, %d endpoints)  \n\n", watchdogHz, endpointCount)
	fmt.Fprintf(&b, "---\n\n")

	fmt.Fprintf(&b, "## GSN Safety Argument\n\n")

	fmt.Fprintf(&b, "**Claim (C-1):** go-RCP command delivery over the request.Dispatcher/udp.Controller "+
		"request/response path meets the %d µs watchdog half-period latency budget under sustained GC pressure.\n\n", threshold)

	fmt.Fprintf(&b, "**Goal (G-1):** Demonstrate that Max(Send latency) < %d µs over a %v "+
		"workload with ~64 MiB/s GC allocation pressure, with ≥ 10 000 samples.\n\n", threshold, dur)

	fmt.Fprintf(&b, "**Strategy (S-1):** Empirical measurement over a real loopback UDP socket "+
		"(udp.Server/udp.Router serving one request.Dispatcher per endpoint, driven by udp.Controller.Write) "+
		"under realistic GC load. Latency sampled for every `Write` call, end-to-end including request "+
		"encode, socket round trip, server-side Dispatch (Submit+Pump+Response), and response decode. "+
		"Percentiles computed over the full sample set.\n\n")

	fmt.Fprintf(&b, "**Evidence (E-1):**\n\n")
	fmt.Fprintf(&b, "### Send latency (udp.Controller.Write, %d samples)\n\n", nSend)
	fmt.Fprintf(&b, "| Metric | Measured | Threshold | Result |\n")
	fmt.Fprintf(&b, "|--------|----------|-----------|--------|\n")
	fmt.Fprintf(&b, "| P50    | %d µs   | —         | —      |\n", time.Duration(p50s).Microseconds())
	fmt.Fprintf(&b, "| P99    | %d µs   | —         | —      |\n", time.Duration(p99s).Microseconds())
	fmt.Fprintf(&b, "| P99.9  | %d µs   | —         | —      |\n", time.Duration(p999s).Microseconds())
	fmt.Fprintf(&b, "| Max    | %d µs   | < %d µs  | **%s** |\n\n", maxSus, threshold, pass)

	fmt.Fprintf(&b, "### GC stop-the-world pauses\n\n")
	fmt.Fprintf(&b, "| Metric            | Value      |\n")
	fmt.Fprintf(&b, "|-------------------|------------|\n")
	fmt.Fprintf(&b, "| GC cycles         | %d         |\n", gcRuns)
	fmt.Fprintf(&b, "| Max STW pause     | %d µs      |\n", time.Duration(maxPauseNs).Microseconds())
	fmt.Fprintf(&b, "| Total STW time    | %d µs      |\n\n", time.Duration(totalPauseNs).Microseconds())

	fmt.Fprintf(&b, "**Assumptions (A-1):**\n\n")
	fmt.Fprintf(&b, "- This measures a real UDP/IP loopback socket (`127.0.0.1`), not the in-process mock the pre-TC18 evidence used — loopback network-stack overhead is included, real physical-network latency is not.\n")
	fmt.Fprintf(&b, "- GOMAXPROCS is at the OS default; CPU isolation (cgroups + IRQ affinity) can further reduce jitter.\n")
	fmt.Fprintf(&b, "- Each endpoint's request.Dispatcher uses a fresh request.Sequencer and no AccessCheck; a deployment with a configured SafeStateCheck or AccessCheck will see additional per-request overhead not captured here.\n\n")

	fmt.Fprintf(&b, "**Residual risk (R-1):**\n\n")
	fmt.Fprintf(&b, "- Real RC Servers operate over a physical Ethernet network (with or without TSN), adding non-deterministic latency beyond loopback.\n")
	fmt.Fprintf(&b, "- GC STW pauses of > 1 ms are possible under extreme allocation rates.\n")
	fmt.Fprintf(&b, "- This evidence covers the udp transport only; a future TSN or shmem transport (see ROADMAP.md) requires its own latency characterisation before integration.\n")
	return b.String()
}
