//fusa:test REQ-ZG-001
//fusa:test REQ-ZG-002
//fusa:test REQ-ZG-003
//fusa:test REQ-ZG-004
//fusa:test REQ-ZG-005
//fusa:test REQ-ZG-006
//fusa:test REQ-ZG-007
//fusa:test REQ-ZG-008

package zonegroup_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
	"github.com/SoundMatt/go-RCP/v9/zonegroup"
)

type stubHandler struct {
	fail bool
}

func (h stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	if h.fail {
		return acf.Message{}, errBoom
	}
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

var errBoom = errors.New("boom")

const testEndpoint = avtp.ByteBusID(1)

func newMember(t *testing.T, suffix uint16, fail bool) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testEndpoint, stubHandler{fail: fail}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, byte(suffix)}, suffix), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, byte(suffix)}, suffix), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// TestZoneGroup_BroadcastAll delivers the request to every member
// concurrently and collects all responses (REQ-ZG-001).
func TestZoneGroup_BroadcastAll(t *testing.T) {
	a := newMember(t, 1, false)
	b := newMember(t, 2, false)
	g, err := zonegroup.NewGroup([]*udp.Controller{a, b})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	res, err := g.Read(context.Background(), testEndpoint)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(res.Results))
	}
	if !res.OK() {
		t.Errorf("OK() = false, want true: %v", res.Errors())
	}
}

// TestZoneGroup_MemberStreamRecorded each result carries the member's own
// StreamID, so a broadcast to several addressable members is distinguishable
// per member (REQ-ZG-002).
func TestZoneGroup_MemberStreamRecorded(t *testing.T) {
	a := newMember(t, 1, false)
	g, err := zonegroup.NewGroup([]*udp.Controller{a})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	res, err := g.Read(context.Background(), testEndpoint)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if res.Results[0].Stream != a.StreamID() {
		t.Errorf("Results[0].Stream = %v, want %v", res.Results[0].Stream, a.StreamID())
	}
}

// TestZoneGroup_PartialFailure OK is false if any member fails, and all
// results are still collected (REQ-ZG-003).
func TestZoneGroup_PartialFailure(t *testing.T) {
	good := newMember(t, 1, false)
	bad := newMember(t, 2, true)
	g, err := zonegroup.NewGroup([]*udp.Controller{good, bad})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	res, err := g.Read(context.Background(), testEndpoint)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(res.Results))
	}
	if res.OK() {
		t.Errorf("OK() = true, want false (one member failed)")
	}
	if len(res.Errors()) != 1 {
		t.Errorf("len(Errors()) = %d, want 1", len(res.Errors()))
	}
}

// TestZoneGroup_ContextCancellation a cancelled context is forwarded to
// every member Request (REQ-ZG-004).
func TestZoneGroup_ContextCancellation(t *testing.T) {
	a := newMember(t, 1, false)
	b := newMember(t, 2, false)
	g, err := zonegroup.NewGroup([]*udp.Controller{a, b})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := g.Read(ctx, testEndpoint)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, mr := range res.Results {
		if !errors.Is(mr.Err, udp.ErrTimeout) {
			t.Errorf("member %v err = %v, want ErrTimeout from a cancelled context", mr.Stream, mr.Err)
		}
	}
}

// TestZoneGroup_StreamIDsAndLen report group membership (REQ-ZG-005).
func TestZoneGroup_StreamIDsAndLen(t *testing.T) {
	a := newMember(t, 1, false)
	b := newMember(t, 2, false)
	g, err := zonegroup.NewGroup([]*udp.Controller{a, b})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	if g.Len() != 2 {
		t.Errorf("Len() = %d, want 2", g.Len())
	}
	ids := g.StreamIDs()
	if len(ids) != 2 || ids[0] != a.StreamID() || ids[1] != b.StreamID() {
		t.Errorf("StreamIDs() = %v, want [%v %v]", ids, a.StreamID(), b.StreamID())
	}
}

// TestZoneGroup_NewGroupRejectsInvalid rejects an empty or nil-containing
// member list (REQ-ZG-006).
func TestZoneGroup_NewGroupRejectsInvalid(t *testing.T) {
	if _, err := zonegroup.NewGroup(nil); err == nil {
		t.Error("NewGroup(nil) = nil error, want error")
	}
	if _, err := zonegroup.NewGroup([]*udp.Controller{nil}); err == nil {
		t.Error("NewGroup([nil]) = nil error, want error")
	}
}

// TestZoneGroup_Close_Idempotent Close closes every member exactly once and
// is safe to call multiple times; Broadcast after Close returns ErrClosed
// (REQ-ZG-007).
func TestZoneGroup_Close_Idempotent(t *testing.T) {
	a := newMember(t, 1, false)
	g, err := zonegroup.NewGroup([]*udp.Controller{a})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}

	if err := g.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := g.Read(context.Background(), testEndpoint); !errors.Is(err, udp.ErrClosed) {
		t.Errorf("Read after Close err = %v, want ErrClosed", err)
	}
}

// TestZoneGroup_ConcurrentBroadcasts multiple goroutines may call Broadcast
// simultaneously without a data race (REQ-ZG-008).
func TestZoneGroup_ConcurrentBroadcasts(t *testing.T) {
	a := newMember(t, 1, false)
	b := newMember(t, 2, false)
	g, err := zonegroup.NewGroup([]*udp.Controller{a, b})
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = g.Read(ctx, testEndpoint)
		}()
	}
	wg.Wait()
}
