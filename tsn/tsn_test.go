//fusa:test REQ-TSN-001
//fusa:test REQ-TSN-002
//fusa:test REQ-TSN-003
//fusa:test REQ-TSN-004
//fusa:test REQ-TSN-005
//fusa:test REQ-TSN-006

package tsn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/tsn"
	"github.com/SoundMatt/go-RCP/udp"
)

func tsnClientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 2)
}

func tsnServerStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 2)
}

// newTSNTestServer starts a udp.Server whose root client is tsnClientStream,
// with endpoint 1 declared and answered by a fixed-response stub Handler.
func newTSNTestServer(t *testing.T) (*udp.Server, *stubHandler) {
	t.Helper()
	root := tsnClientStream()
	srv := server.NewServer()
	if err := srv.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)
	h := &stubHandler{body: []byte{0x2A}}
	if err := router.Register(1, h); err != nil {
		t.Fatalf("Register: %v", err)
	}
	us, err := udp.NewServer(tsnServerStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = us.Close() })
	return us, h
}

// stubHandler answers every request with a fixed body.
type stubHandler struct {
	body []byte
}

func (h *stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           h.body,
	}, nil
}

// TestDefaultPCPMap_MapsRanksToDescendingPCP verifies the default map
// assigns strictly descending PCP values from rank 0 (highest priority,
// cancellation) to rank 6 (lowest, plain) (REQ-TSN-001).
func TestDefaultPCPMap_MapsRanksToDescendingPCP(t *testing.T) {
	m := tsn.DefaultPCPMap()
	prev := uint8(8)
	for rank := 0; rank < 7; rank++ {
		pcp := m[rank]
		if pcp >= prev {
			t.Errorf("rank %d PCP = %d, want strictly less than previous rank's %d", rank, pcp, prev)
		}
		prev = pcp
	}
}

// TestController_PCPFor_ReturnsConfiguredPCP verifies PCPFor reflects the
// configured PCPMap for a representative set of request.Kind values
// (REQ-TSN-002).
func TestController_PCPFor_ReturnsConfiguredPCP(t *testing.T) {
	cfg := tsn.DefaultConfig()
	ctrl, err := tsn.NewController(tsnClientStream(), "127.0.0.1:1", cfg)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = ctrl.Close() }()

	if got := ctrl.PCPFor(request.KindPlain); got != cfg.PCPMap[request.KindPlain.Priority()] {
		t.Errorf("PCPFor(KindPlain) = %d, want %d", got, cfg.PCPMap[request.KindPlain.Priority()])
	}
	if got := ctrl.PCPFor(request.KindCancelAll); got != cfg.PCPMap[request.KindCancelAll.Priority()] {
		t.Errorf("PCPFor(KindCancelAll) = %d, want %d", got, cfg.PCPMap[request.KindCancelAll.Priority()])
	}
}

// TestController_Read_DeliversViaUDP verifies a TSN Controller's Read
// reaches the registered Handler and returns its response (REQ-TSN-003).
func TestController_Read_DeliversViaUDP(t *testing.T) {
	us, _ := newTSNTestServer(t)
	ctrl, err := tsn.NewController(tsnClientStream(), us.Addr().String(), tsn.DefaultConfig())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = ctrl.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := ctrl.Read(ctx, 1)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(resp.Body) != 1 || resp.Body[0] != 0x2A {
		t.Errorf("Body = % X, want [2A]", resp.Body)
	}
}

// TestController_Write_HighPriorityRequest verifies a Write issued with the
// highest-priority Kind (cancellation) still delivers successfully — i.e.
// setSocketPriority never breaks the send path even on platforms where it
// is a no-op (REQ-TSN-004).
func TestController_Write_HighPriorityRequest(t *testing.T) {
	us, _ := newTSNTestServer(t)
	ctrl, err := tsn.NewController(tsnClientStream(), us.Addr().String(), tsn.DefaultConfig())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = ctrl.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := ctrl.Request(ctx, 1, acf.FlagWrite, []byte{0x01}, request.KindCancelAll)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("response missing FlagResponse")
	}
}

// TestController_Close_RejectsFurtherRequests verifies a closed Controller
// rejects a subsequent request (REQ-TSN-005).
func TestController_Close_RejectsFurtherRequests(t *testing.T) {
	us, _ := newTSNTestServer(t)
	ctrl, err := tsn.NewController(tsnClientStream(), us.Addr().String(), tsn.DefaultConfig())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	_ = ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = ctrl.Read(ctx, 1)
	if !errors.Is(err, udp.ErrClosed) {
		t.Errorf("error = %v, want ErrClosed", err)
	}
}

// TestConfig_PreservesVLANAndCycle verifies Config.VLAN/CycleNs survive
// unmodified through NewController/Config (REQ-TSN-006).
func TestConfig_PreservesVLANAndCycle(t *testing.T) {
	cfg := tsn.Config{VLAN: 42, CycleNs: 123456, PCPMap: tsn.DefaultPCPMap()}
	ctrl, err := tsn.NewController(tsnClientStream(), "127.0.0.1:1", cfg)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = ctrl.Close() }()

	got := ctrl.Config()
	if got.VLAN != 42 || got.CycleNs != 123456 {
		t.Errorf("Config() = %+v, want VLAN=42 CycleNs=123456", got)
	}
}
