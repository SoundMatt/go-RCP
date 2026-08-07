package rcp_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/mock"
)

// testAddr is the byte_bus_id every helper in this file's fixture answers
// requests for; its decimal string ("1") is what rcp.EndpointIDString(1)
// produces and what test relay.Message values use as their ID.
const testAddr avtp.ByteBusID = 1

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x74, 0x65, 0x73, 0x74, 0x00}, 1)
}

// newTestController returns a *mock.Fixture and its root *mock.Client,
// wired to a single in-process endpoint at testAddr. fn customizes the
// endpoint's response; a nil fn answers every request with FlagResponse
// (echoing the originating Read/Write flag) and body "ack".
func newTestController(t *testing.T, fn mock.EndpointFunc) (*mock.Fixture, *mock.Client) {
	t.Helper()
	fx, err := mock.NewFixture(testStream(), false)
	if err != nil {
		t.Fatalf("NewFixture: %v", err)
	}
	t.Cleanup(func() { _ = fx.Close() })
	if fn == nil {
		fn = func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
			return acf.Message{
				Kind:           req.Kind,
				ByteBusID:      req.ByteBusID,
				TransactionNum: req.TransactionNum,
				Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
				Body:           []byte("ack"),
			}, nil
		}
	}
	if err := fx.Router.Register(testAddr, mock.NewEndpoint(testAddr, fn)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return fx, fx.Root
}
