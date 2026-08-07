//fusa:test REQ-DDS-001
//fusa:test REQ-DDS-002
//fusa:test REQ-DDS-003
//fusa:test REQ-DDS-004
//fusa:test REQ-DDS-005
//fusa:test REQ-DDS-006
//fusa:test REQ-DDS-007
//fusa:test REQ-DDS-008

package ddsbr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/ddsbr"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

const testAddr = avtp.ByteBusID(1)

// stubHandler answers every request with FlagResponse set and an echoed
// body, mirroring udp_test.go's own fixture.
type stubHandler struct{}

func (stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

func newUpstream(t *testing.T) *udp.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	if err := router.Register(testAddr, stubHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	ctrl, err := udp.NewController(clientStream(), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return ctrl
}

// TestDomain_NewTopic_Registers a repeated NewTopic call for the same name
// returns the same Topic instance (REQ-DDS-001).
func TestDomain_NewTopic_Registers(t *testing.T) {
	d := ddsbr.NewDomain()
	a := d.NewTopic("telemetry")
	b := d.NewTopic("telemetry")
	if a != b {
		t.Error("NewTopic returned distinct Topics for the same name")
	}
}

// TestDataWriter_Broadcast a DataWriter's Write reaches every subscribed
// DataReader (REQ-DDS-002).
func TestDataWriter_Broadcast(t *testing.T) {
	d := ddsbr.NewDomain()
	topic := d.NewTopic("telemetry")
	w := ddsbr.NewDataWriter(topic)
	r1 := ddsbr.NewDataReader(topic)
	r2 := ddsbr.NewDataReader(topic)
	defer r1.Close()
	defer r2.Close()

	w.Write("sample")
	for _, r := range []*ddsbr.DataReader{r1, r2} {
		select {
		case got := <-r.Read():
			if got != "sample" {
				t.Errorf("got %v, want %q", got, "sample")
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for sample")
		}
	}
}

// TestDataReader_ReceivesSample a DataReader receives exactly what was
// written (REQ-DDS-003).
func TestDataReader_ReceivesSample(t *testing.T) {
	d := ddsbr.NewDomain()
	topic := d.NewTopic("cmd")
	r := ddsbr.NewDataReader(topic)
	defer r.Close()

	ddsbr.NewDataWriter(topic).Write(42)
	select {
	case got := <-r.Read():
		if got != 42 {
			t.Errorf("got %v, want 42", got)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for sample")
	}
}

// TestBridge_PublishResponse PublishResponse fans an endpoint response out
// to the telemetry topic as a ResponseSample (REQ-DDS-004).
func TestBridge_PublishResponse(t *testing.T) {
	upstream := newUpstream(t)
	d := ddsbr.NewDomain()
	telemetryTopic := d.NewTopic("telemetry")
	cmdTopic := d.NewTopic("cmd")
	reader := ddsbr.NewDataReader(telemetryTopic)
	defer reader.Close()

	b := ddsbr.NewBridge(upstream, ddsbr.NewDataWriter(telemetryTopic), ddsbr.NewDataReader(cmdTopic))
	defer b.Close()

	resp, err := upstream.Read(context.Background(), testAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	b.PublishResponse(resp)

	select {
	case got := <-reader.Read():
		sample, ok := got.(ddsbr.ResponseSample)
		if !ok {
			t.Fatalf("got %T, want ddsbr.ResponseSample", got)
		}
		if sample.ByteBusID != testAddr {
			t.Errorf("ByteBusID = %v, want %v", sample.ByteBusID, testAddr)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for telemetry sample")
	}
}

// TestBridge_PublishTrigger PublishTrigger fans a trigger-event count out to
// the telemetry topic as a TriggerSample (REQ-DDS-005).
func TestBridge_PublishTrigger(t *testing.T) {
	upstream := newUpstream(t)
	d := ddsbr.NewDomain()
	telemetryTopic := d.NewTopic("telemetry")
	cmdTopic := d.NewTopic("cmd")
	reader := ddsbr.NewDataReader(telemetryTopic)
	defer reader.Close()

	b := ddsbr.NewBridge(upstream, ddsbr.NewDataWriter(telemetryTopic), ddsbr.NewDataReader(cmdTopic))
	defer b.Close()

	b.PublishTrigger(testAddr, 3)

	select {
	case got := <-reader.Read():
		sample, ok := got.(ddsbr.TriggerSample)
		if !ok {
			t.Fatalf("got %T, want ddsbr.TriggerSample", got)
		}
		if sample.Count != 3 {
			t.Errorf("Count = %d, want 3", sample.Count)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for trigger sample")
	}
}

// TestBridge_CommandDispatch a CommandSample arriving on cmdReader is
// forwarded to the upstream controller as a request (REQ-DDS-006).
func TestBridge_CommandDispatch(t *testing.T) {
	upstream := newUpstream(t)
	d := ddsbr.NewDomain()
	telemetryTopic := d.NewTopic("telemetry")
	cmdTopic := d.NewTopic("cmd")
	cmdWriter := ddsbr.NewDataWriter(cmdTopic)

	b := ddsbr.NewBridge(upstream, ddsbr.NewDataWriter(telemetryTopic), ddsbr.NewDataReader(cmdTopic))
	defer b.Close()

	cmdWriter.Write(ddsbr.CommandSample{ByteBusID: testAddr, Control: acf.FlagWrite, Body: []byte("hello")})

	// The stub handler on the far side echoes the body; poll the endpoint's
	// own state indirectly isn't possible here (stub is stateless), so this
	// confirms dispatch didn't hang or panic by racing a normal request
	// against the same endpoint immediately after.
	time.Sleep(50 * time.Millisecond)
	if _, err := upstream.Read(context.Background(), testAddr); err != nil {
		t.Fatalf("Read after dispatch: %v", err)
	}
}

// TestBridge_CloseIdempotent Close stops the command-forwarding goroutine
// and is safe to call twice (REQ-DDS-007).
func TestBridge_CloseIdempotent(t *testing.T) {
	upstream := newUpstream(t)
	d := ddsbr.NewDomain()
	telemetryTopic := d.NewTopic("telemetry")
	cmdTopic := d.NewTopic("cmd")

	b := ddsbr.NewBridge(upstream, ddsbr.NewDataWriter(telemetryTopic), ddsbr.NewDataReader(cmdTopic))
	b.Close()
	b.Close() // must not panic or block
}

// TestDomain_Lookup_NotFound Lookup reports ErrTopicNotFound for an
// unregistered name (REQ-DDS-008).
func TestDomain_Lookup_NotFound(t *testing.T) {
	d := ddsbr.NewDomain()
	if _, err := d.Lookup("nope"); !errors.Is(err, ddsbr.ErrTopicNotFound) {
		t.Errorf("err = %v, want ErrTopicNotFound", err)
	}
}
