//fusa:test REQ-UDP-014

package udp_test

import (
	"context"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

// TestEP0Handler_TimedRead_Rejected verifies a discovery-shaped EP0 read
// framed in a timestamped (TSCF) header is answered with a wire-level
// error rather than the register map, per server.Server.HandleDiscoveryRequest's
// own untimed-only rule (REQ-UDP-014).
func TestEP0Handler_TimedRead_Rejected(t *testing.T) {
	h := udp.NewEP0Handler(server.NewServer())
	hdr := avtp.Header{Timed: true, TimestampStatus: avtp.TimestampValid}
	req := acf.Message{ByteBusID: regmap.EP0, Control: acf.FlagRead}

	_, err := h.HandleRequest(hdr, req)
	if err == nil {
		t.Fatalf("HandleRequest: want error for timed discovery read, got nil")
	}
}

// TestEP0Handler_WriteRequiresRoot verifies an EP0 write from a stream that
// never claimed root is rejected, exercised end-to-end through a Router and
// live udp.Server/Controller pair.
func TestEP0Handler_WriteRequiresRoot(t *testing.T) {
	srv := server.NewServer()
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)
	us, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer func() { _ = us.Close() }()

	ctrl := dial(t, us, clientStream())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Write(ctx, regmap.EP0, []byte{0x00})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Errorf("response missing FlagError for non-root EP0 write")
	}
}
