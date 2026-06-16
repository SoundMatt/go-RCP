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

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/ddsbr"
	"github.com/SoundMatt/go-RCP/mock"
)

// REQ-DDS-001: Domain.NewTopic creates and registers a named topic; Lookup retrieves it.
func TestDomain_NewTopicAndLookup(t *testing.T) {
	d := ddsbr.NewDomain()
	topic := d.NewTopic("rcp/status")
	if topic == nil {
		t.Fatal("NewTopic returned nil")
	}
	if topic.Name() != "rcp/status" {
		t.Errorf("Name = %q, want rcp/status", topic.Name())
	}
	got, err := d.Lookup("rcp/status")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != topic {
		t.Error("Lookup returned different topic")
	}
}

// REQ-DDS-008: Domain.Lookup returns ErrTopicNotFound for unknown names.
func TestDomain_Lookup_NotFound(t *testing.T) {
	d := ddsbr.NewDomain()
	_, err := d.Lookup("nonexistent")
	if !errors.Is(err, ddsbr.ErrTopicNotFound) {
		t.Errorf("want ErrTopicNotFound, got %v", err)
	}
}

// REQ-DDS-002: Topic.Write delivers a sample to all subscribers.
func TestTopic_Write_Broadcast(t *testing.T) {
	d := ddsbr.NewDomain()
	topic := d.NewTopic("test")

	r1 := ddsbr.NewDataReader(topic)
	defer r1.Close()
	r2 := ddsbr.NewDataReader(topic)
	defer r2.Close()

	w := ddsbr.NewDataWriter(topic)
	w.Write("hello")

	for _, r := range []*ddsbr.DataReader{r1, r2} {
		select {
		case got := <-r.Read():
			if got.(string) != "hello" {
				t.Errorf("got %v, want hello", got)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for sample")
		}
	}
}

// REQ-DDS-003: DataReader receives samples written by DataWriter.
func TestDataWriter_DataReader_RoundTrip(t *testing.T) {
	d := ddsbr.NewDomain()
	topic := d.NewTopic("cmd")
	w := ddsbr.NewDataWriter(topic)
	r := ddsbr.NewDataReader(topic)
	defer r.Close()

	w.Write(42)
	select {
	case v := <-r.Read():
		if v.(int) != 42 {
			t.Errorf("got %v, want 42", v)
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

// REQ-DDS-004: Bridge publishes rcp.Status to the status topic.
func TestBridge_PublishStatus(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	d := ddsbr.NewDomain()
	statusTopic := d.NewTopic("rcp/status")
	cmdTopic := d.NewTopic("rcp/cmd")

	sw := ddsbr.NewDataWriter(statusTopic)
	sr := ddsbr.NewDataReader(statusTopic)
	defer sr.Close()
	cr := ddsbr.NewDataReader(cmdTopic)
	defer cr.Close()

	b := ddsbr.NewBridge(inner, sw, cr)
	defer b.Close()

	// Publish a status update from the mock.
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			inner.Publish([]byte("status-data"))
		}
	}()

	select {
	case sample := <-sr.Read():
		st, ok := sample.(*rcp.Status)
		if !ok {
			t.Fatalf("expected *rcp.Status, got %T", sample)
		}
		if st.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone = %v, want ZoneFrontLeft", st.Zone)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for status sample")
	}
}

// REQ-DDS-005: Bridge dispatches DDS command samples to the controller.
func TestBridge_SubscribeCommands(t *testing.T) {
	dispatched := make(chan rcp.CommandType, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	d := ddsbr.NewDomain()
	statusTopic := d.NewTopic("rcp/status")
	cmdTopic := d.NewTopic("rcp/cmd")

	sw := ddsbr.NewDataWriter(statusTopic)
	cw := ddsbr.NewDataWriter(cmdTopic)
	cr := ddsbr.NewDataReader(cmdTopic)
	defer cr.Close()

	b := ddsbr.NewBridge(inner, sw, cr)
	defer b.Close()

	cw.Write(&rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Priority: rcp.PriorityNormal})

	select {
	case got := <-dispatched:
		if got != rcp.CmdSet {
			t.Errorf("type = %v, want CmdSet", got)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for command dispatch")
	}
}

// REQ-DDS-006: Bridge.Close stops status publication.
func TestBridge_Close_StopsGoroutines(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	d := ddsbr.NewDomain()
	sw := ddsbr.NewDataWriter(d.NewTopic("s"))
	cr := ddsbr.NewDataReader(d.NewTopic("c"))
	defer cr.Close()

	b := ddsbr.NewBridge(inner, sw, cr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		b.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Error("Bridge.Close timed out")
	}
}

// REQ-DDS-007: Bridge.Close is idempotent.
func TestBridge_CloseIdempotent(t *testing.T) {
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	d := ddsbr.NewDomain()
	sw := ddsbr.NewDataWriter(d.NewTopic("s2"))
	cr := ddsbr.NewDataReader(d.NewTopic("c2"))
	defer cr.Close()

	b := ddsbr.NewBridge(inner, sw, cr)
	b.Close()
	b.Close() // must not panic or hang
}
