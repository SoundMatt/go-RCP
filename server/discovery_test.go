//fusa:test REQ-RCS-021
//fusa:test REQ-RCS-022
//fusa:test REQ-RCS-023
//fusa:test REQ-RCS-024
//fusa:test REQ-RCS-025
//fusa:test REQ-RCS-026
//fusa:test REQ-RCS-027
//fusa:test REQ-RCS-028
//fusa:test REQ-RCS-029
//fusa:test REQ-RCS-030

package server_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/discovery"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/server"
)

// fakeClock provides a controllable clock for testing the Discovery
// configuration-claim timeout without real-time sleeps, mirroring
// ratelimit's own fakeClock test helper.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Now()}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func otherStream(suffix byte) avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, suffix}, uint16(suffix))
}

// TestReadDiscovery_AnswersInEveryLifecycleState checks a restricted stream
// with no grant at all can still read via discovery in every LifecycleState
// (REQ-RCS-021).
func TestReadDiscovery_AnswersInEveryLifecycleState(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	restricted := otherStream(0xB0)

	// StateUnconfigured.
	buf := s.ReadDiscovery()
	if _, err := regmap.DecodeRegisterMap(buf); err != nil {
		t.Fatalf("DecodeRegisterMap (unconfigured): %v", err)
	}

	advanceToHWLocked(t, s, root, 1)
	if err := s.WriteFunctional(root, 1, []byte{0x01}); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	if err := s.SetQueueConfig(root, regmap.QueueConfig{FlushThreshold: 4}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}

	// StateHWLocked.
	buf = s.ReadDiscovery()
	if _, err := regmap.DecodeRegisterMap(buf); err != nil {
		t.Fatalf("DecodeRegisterMap (hw-locked): %v", err)
	}

	if err := s.AdvanceToFullyConfigured(); err != nil {
		t.Fatalf("AdvanceToFullyConfigured: %v", err)
	}

	// StateFullyConfigured, exercised by a stream with no grant at all —
	// ReadEP0 would deny this, ReadDiscovery must not.
	if _, err := s.ReadEP0(restricted); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Fatalf("ReadEP0(restricted) = %v, want ErrAccessDenied (sanity check)", err)
	}
	buf = s.ReadDiscovery()
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap (fully-configured): %v", err)
	}
	if _, ok := m.Endpoint(1); !ok {
		t.Errorf("discovery read missing endpoint 1")
	}
}

// TestReadDiscovery_AvailableWhileConfigurationClaimHeld checks discovery
// reads from other streams are never blocked by an active configuration
// claim (REQ-RCS-022).
func TestReadDiscovery_AvailableWhileConfigurationClaimHeld(t *testing.T) {
	s := server.NewServer()
	holder := otherStream(0xC1)
	reader := otherStream(0xC2)

	if err := s.ClaimConfiguration(holder); err != nil {
		t.Fatalf("ClaimConfiguration(holder): %v", err)
	}

	buf := s.ReadDiscovery()
	if _, err := regmap.DecodeRegisterMap(buf); err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	_ = reader // reader never needs a grant/claim of its own to read
}

// TestHandleDiscoveryRequest_RejectsTimedHeader checks a discovery request
// framed in a presentation-timestamped (TSCF) header is dropped rather than
// answered (REQ-RCS-023).
func TestHandleDiscoveryRequest_RejectsTimedHeader(t *testing.T) {
	s := server.NewServer()
	hdr := avtp.Header{Timed: true, TimestampStatus: avtp.TimestampValid}

	buf, err := s.HandleDiscoveryRequest(hdr, false)
	if !errors.Is(err, discovery.ErrDiscoveryRequiresUntimedHeader) {
		t.Fatalf("HandleDiscoveryRequest(timed) err = %v, want ErrDiscoveryRequiresUntimedHeader", err)
	}
	if buf != nil {
		t.Errorf("HandleDiscoveryRequest(timed) buf = % X, want nil", buf)
	}
}

// TestHandleDiscoveryRequest_RejectsACFGBB checks a discovery read framed as
// ACF_GBB is dropped rather than answered, per TC18 §12.6.1 Table 16 ("as
// well as requests in ACF_GBB format").
func TestHandleDiscoveryRequest_RejectsACFGBB(t *testing.T) {
	s := server.NewServer()
	hdr := avtp.Header{Timed: false}

	buf, err := s.HandleDiscoveryRequest(hdr, true)
	if !errors.Is(err, discovery.ErrDiscoveryRequestIsACFGBB) {
		t.Fatalf("HandleDiscoveryRequest(ACF_GBB) err = %v, want ErrDiscoveryRequestIsACFGBB", err)
	}
	if buf != nil {
		t.Errorf("HandleDiscoveryRequest(ACF_GBB) buf = % X, want nil", buf)
	}
}

// TestHandleDiscoveryRequest_AnswersUntimedHeader checks a discovery
// request framed in the untimed (NTSCF) header is answered with the same
// bytes ReadDiscovery would return (REQ-RCS-024).
func TestHandleDiscoveryRequest_AnswersUntimedHeader(t *testing.T) {
	s := server.NewServer()
	hdr := avtp.Header{Timed: false}

	buf, err := s.HandleDiscoveryRequest(hdr, false)
	if err != nil {
		t.Fatalf("HandleDiscoveryRequest(untimed): %v", err)
	}
	if want := s.ReadDiscovery(); !bytes.Equal(buf, want) {
		t.Errorf("HandleDiscoveryRequest(untimed) = % X, want % X", buf, want)
	}
}

// TestClaimConfiguration_FirstStreamReservesAndBlocksOthers checks the first
// caller reserves configuration rights and a different stream is denied
// while that reservation is unexpired (REQ-RCS-025).
func TestClaimConfiguration_FirstStreamReservesAndBlocksOthers(t *testing.T) {
	fc := newFakeClock()
	s := server.NewServerWithClock(fc.Now)
	first := otherStream(0xD1)
	second := otherStream(0xD2)

	if err := s.ClaimConfiguration(first); err != nil {
		t.Fatalf("ClaimConfiguration(first): %v", err)
	}
	if err := s.ClaimConfiguration(second); !errors.Is(err, discovery.ErrConfigurationClaimed) {
		t.Fatalf("ClaimConfiguration(second) = %v, want ErrConfigurationClaimed", err)
	}

	claimant, ok := s.ConfigurationClaimant()
	if !ok || claimant != first {
		t.Errorf("ConfigurationClaimant() = (%v, %v), want (%v, true)", claimant, ok, first)
	}
}

// TestClaimConfiguration_RenewsForSameStream checks a re-claim by the
// current holder is idempotent and extends the reservation, modelling a
// "follow-up configuration request" (REQ-RCS-026).
func TestClaimConfiguration_RenewsForSameStream(t *testing.T) {
	fc := newFakeClock()
	s := server.NewServerWithClock(fc.Now)
	s.SetConfigurationClaimTimeout(10 * time.Second)
	stream := otherStream(0xE1)

	if err := s.ClaimConfiguration(stream); err != nil {
		t.Fatalf("ClaimConfiguration: %v", err)
	}

	fc.Advance(7 * time.Second)
	if err := s.ClaimConfiguration(stream); err != nil {
		t.Fatalf("ClaimConfiguration (renewal): %v", err)
	}

	// Without the renewal the claim would have expired by now (7s + 7s >
	// 10s); the renewal must have reset the window from the second call.
	fc.Advance(7 * time.Second)
	if _, ok := s.ConfigurationClaimant(); !ok {
		t.Errorf("ConfigurationClaimant() reports no active claim, want the renewal to still be in effect")
	}
}

// TestClaimConfiguration_ExpiredClaimIsReclaimable checks a different stream
// can claim configuration rights once the current holder's timeout has
// lapsed with no follow-up request (REQ-RCS-027).
func TestClaimConfiguration_ExpiredClaimIsReclaimable(t *testing.T) {
	fc := newFakeClock()
	s := server.NewServerWithClock(fc.Now)
	s.SetConfigurationClaimTimeout(5 * time.Second)
	first := otherStream(0xF1)
	second := otherStream(0xF2)

	if err := s.ClaimConfiguration(first); err != nil {
		t.Fatalf("ClaimConfiguration(first): %v", err)
	}

	fc.Advance(5*time.Second + time.Millisecond)

	if _, ok := s.ConfigurationClaimant(); ok {
		t.Errorf("ConfigurationClaimant() reports an active claim after timeout elapsed, want none")
	}
	if err := s.ClaimConfiguration(second); err != nil {
		t.Fatalf("ClaimConfiguration(second) after expiry: %v", err)
	}
	claimant, ok := s.ConfigurationClaimant()
	if !ok || claimant != second {
		t.Errorf("ConfigurationClaimant() = (%v, %v), want (%v, true)", claimant, ok, second)
	}
}

// TestReleaseConfigurationClaim_HolderAndNonHolder checks the holder can
// release its own claim early, and a non-holder is rejected (REQ-RCS-028).
func TestReleaseConfigurationClaim_HolderAndNonHolder(t *testing.T) {
	fc := newFakeClock()
	s := server.NewServerWithClock(fc.Now)
	holder := otherStream(0x01)
	other := otherStream(0x02)

	if err := s.ClaimConfiguration(holder); err != nil {
		t.Fatalf("ClaimConfiguration(holder): %v", err)
	}

	if err := s.ReleaseConfigurationClaim(other); !errors.Is(err, discovery.ErrNotConfigurationClaimant) {
		t.Fatalf("ReleaseConfigurationClaim(other) = %v, want ErrNotConfigurationClaimant", err)
	}
	if err := s.ReleaseConfigurationClaim(holder); err != nil {
		t.Fatalf("ReleaseConfigurationClaim(holder): %v", err)
	}
	if _, ok := s.ConfigurationClaimant(); ok {
		t.Errorf("ConfigurationClaimant() reports a claim after release, want none")
	}
	// The claim is now free for anyone, including a stream other than the
	// original holder.
	if err := s.ClaimConfiguration(other); err != nil {
		t.Fatalf("ClaimConfiguration(other) after release: %v", err)
	}
}

// TestIsConformantServer checks recognition of a discovery response's
// identification/version/vendor/device fields (REQ-RCS-029).
func TestIsConformantServer(t *testing.T) {
	good := regmap.GeneralBlock{
		Magic:           regmap.GeneralBlockMagic,
		VendorID:        0x1234,
		DeviceID:        0x5678,
		ProtocolVersion: regmap.RegisterMapVersion,
	}
	if !discovery.IsConformantServer(good) {
		t.Errorf("IsConformantServer(good) = false, want true")
	}

	badMagic := good
	badMagic.Magic++
	if discovery.IsConformantServer(badMagic) {
		t.Errorf("IsConformantServer(badMagic) = true, want false")
	}

	badVersion := good
	badVersion.ProtocolVersion = regmap.RegisterMapVersion + 1
	if discovery.IsConformantServer(badVersion) {
		t.Errorf("IsConformantServer(badVersion) = true, want false")
	}

	zeroVendor := good
	zeroVendor.VendorID = 0
	if discovery.IsConformantServer(zeroVendor) {
		t.Errorf("IsConformantServer(zeroVendor) = true, want false")
	}

	zeroDevice := good
	zeroDevice.DeviceID = 0
	if discovery.IsConformantServer(zeroDevice) {
		t.Errorf("IsConformantServer(zeroDevice) = true, want false")
	}
}

// TestTopology_RoundTrips checks DiscoverTopology extracts the expected
// summary from a discovery response, and WriteTopology/ReadTopology
// round-trip it so re-discovery isn't mandatory every power cycle
// (REQ-RCS-030).
func TestTopology_RoundTrips(t *testing.T) {
	root := rootStream()
	s := newRootServer(t, root)
	if err := s.AddEndpoint(root, 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.AddEndpoint(root, 2, regmap.EndpointTypeSPI); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	buf := s.ReadDiscovery()
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}

	topo := discovery.DiscoverTopology(m)
	if topo.EndpointCount() != 2 {
		t.Fatalf("EndpointCount() = %d, want 2", topo.EndpointCount())
	}
	if topo.Endpoints[0].Address != 1 || topo.Endpoints[0].Type != regmap.EndpointTypeGPIO {
		t.Errorf("Endpoints[0] = %+v, want {Address:1 Type:GPIO}", topo.Endpoints[0])
	}
	if topo.Endpoints[1].Address != 2 || topo.Endpoints[1].Type != regmap.EndpointTypeSPI {
		t.Errorf("Endpoints[1] = %+v, want {Address:2 Type:SPI}", topo.Endpoints[1])
	}

	var persisted bytes.Buffer
	if writeErr := discovery.WriteTopology(&persisted, topo); writeErr != nil {
		t.Fatalf("WriteTopology: %v", writeErr)
	}
	reloaded, err := discovery.ReadTopology(&persisted)
	if err != nil {
		t.Fatalf("ReadTopology: %v", err)
	}
	if reloaded.VendorID != topo.VendorID || reloaded.DeviceID != topo.DeviceID ||
		reloaded.ProtocolVersion != topo.ProtocolVersion || len(reloaded.Endpoints) != len(topo.Endpoints) {
		t.Errorf("ReadTopology() = %+v, want %+v", reloaded, topo)
	}
}
