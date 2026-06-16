//fusa:test REQ-REC-001
//fusa:test REQ-REC-002
//fusa:test REQ-REC-003
//fusa:test REQ-REC-004
//fusa:test REQ-REC-005
//fusa:test REQ-REC-006
//fusa:test REQ-REC-007
//fusa:test REQ-REC-008

package record_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/record"
)

func TestRecordCommand(t *testing.T) {
	rec := record.New(0)
	cmd := &rcp.Command{ID: 42, Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Payload: []byte{1, 2, 3}}
	rec.RecordCommand(cmd)

	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("want 1 entry, got %d", len(snap))
	}
	if snap[0].Type != record.TypeCommand {
		t.Errorf("type = %v, want TypeCommand", snap[0].Type)
	}
	if snap[0].Command.ID != 42 {
		t.Errorf("cmd.ID = %d, want 42", snap[0].Command.ID)
	}
}

func TestRecordResponse(t *testing.T) {
	rec := record.New(0)
	resp := &rcp.Response{CommandID: 7, Zone: rcp.ZoneCentral, Status: rcp.StatusOK}
	rec.RecordResponse(resp)

	snap := rec.Snapshot()
	if len(snap) != 1 || snap[0].Type != record.TypeResponse {
		t.Fatalf("want 1 response entry, got %d", len(snap))
	}
	if snap[0].Response.CommandID != 7 {
		t.Errorf("resp.CommandID = %d, want 7", snap[0].Response.CommandID)
	}
}

func TestRecordStatus(t *testing.T) {
	rec := record.New(0)
	s := &rcp.Status{Zone: rcp.ZoneRearLeft, Seq: 5, Healthy: true}
	rec.RecordStatus(s)

	snap := rec.Snapshot()
	if len(snap) != 1 || snap[0].Type != record.TypeStatus {
		t.Fatalf("want 1 status entry, got %d", len(snap))
	}
	if !snap[0].Status.Healthy {
		t.Error("status.Healthy = false, want true")
	}
}

func TestRingBuffer_OverwritesOldest(t *testing.T) {
	rec := record.New(3) // ring of 3
	for i := 0; i < 5; i++ {
		rec.RecordCommand(&rcp.Command{ID: uint32(i), Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	}

	snap := rec.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("ring size 3, want 3 entries, got %d", len(snap))
	}
	// IDs should be 2, 3, 4 (oldest 0,1 overwritten)
	for i, e := range snap {
		want := uint32(i + 2)
		if e.Command.ID != want {
			t.Errorf("snap[%d].ID = %d, want %d", i, e.Command.ID, want)
		}
	}
	if rec.Written() != 5 {
		t.Errorf("Written = %d, want 5", rec.Written())
	}
}

func TestWriteToReadFrom_RoundTrip(t *testing.T) {
	rec := record.New(0)
	rec.RecordCommand(&rcp.Command{ID: 1, Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Payload: []byte("hello")})
	rec.RecordResponse(&rcp.Response{CommandID: 1, Zone: rcp.ZoneFrontLeft, Status: rcp.StatusOK})
	rec.RecordStatus(&rcp.Status{Zone: rcp.ZoneFrontLeft, Seq: 1, Healthy: true})

	var buf bytes.Buffer
	if _, err := rec.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	entries, err := record.ReadFrom(&buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Command.ID != 1 {
		t.Errorf("round-trip cmd ID = %d, want 1", entries[0].Command.ID)
	}
}

func TestReadFrom_CorruptCRC(t *testing.T) {
	rec := record.New(0)
	rec.RecordCommand(&rcp.Command{ID: 99, Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet})

	var buf bytes.Buffer
	if _, err := rec.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	// Flip a byte in the payload area.
	b := buf.Bytes()
	if len(b) > 15 {
		b[15] ^= 0xFF
	}

	_, err := record.ReadFrom(bytes.NewReader(b))
	if !errors.Is(err, record.ErrCorrupted) {
		t.Errorf("want ErrCorrupted, got %v", err)
	}
}

func TestController_RecordsOnSend(t *testing.T) {
	rec := record.New(0)
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := record.NewController(m, rec)

	if _, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}); err != nil {
		t.Fatal(err)
	}

	snap := rec.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("want 2 entries (cmd+resp), got %d", len(snap))
	}
	if snap[0].Type != record.TypeCommand {
		t.Errorf("first entry type = %v, want TypeCommand", snap[0].Type)
	}
	if snap[1].Type != record.TypeResponse {
		t.Errorf("second entry type = %v, want TypeResponse", snap[1].Type)
	}
}

func TestController_RecordsOnSubscribe(t *testing.T) {
	rec := record.New(0)
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := record.NewController(m, rec)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}

	m.Publish([]byte("ping"))
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for status")
	}
	cancel()

	// Drain the channel.
	for range ch {
	}

	snap := rec.Snapshot()
	hasStatus := false
	for _, e := range snap {
		if e.Type == record.TypeStatus {
			hasStatus = true
		}
	}
	if !hasStatus {
		t.Error("expected at least one status entry")
	}
}

func TestReplay_SendsCommands(t *testing.T) {
	rec := record.New(0)
	// Record two commands.
	rec.RecordCommand(&rcp.Command{ID: 1, Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	rec.RecordCommand(&rcp.Command{ID: 2, Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet})
	rec.RecordResponse(&rcp.Response{CommandID: 1, Zone: rcp.ZoneFrontLeft, Status: rcp.StatusOK})

	target := mock.NewController(rcp.ZoneFrontLeft, nil)
	resps, err := record.Replay(context.Background(), rec.Snapshot(), target)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(resps) != 2 {
		t.Errorf("want 2 responses, got %d", len(resps))
	}
}

func TestClose_Idempotent(t *testing.T) {
	rec := record.New(0)
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := record.NewController(m, rec)

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestClose_RejectsSend(t *testing.T) {
	rec := record.New(0)
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := record.NewController(m, rec)
	c.Close()

	_, err := c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("want ErrClosed, got %v", err)
	}
}

func TestConcurrent(t *testing.T) {
	rec := record.New(0)
	m := mock.NewController(rcp.ZoneFrontLeft, nil)
	c := record.NewController(m, rec)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Send(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet}) //nolint:errcheck
		}()
	}
	wg.Wait()
}
