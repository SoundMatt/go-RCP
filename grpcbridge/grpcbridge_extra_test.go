package grpcbridge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/grpcbridge"
)

// Subscribe after Close reports ErrClosed rather than attempting to dial.
func TestController_Subscribe_AfterClose(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := c.Subscribe(ctx); !errors.Is(err, grpcbridge.ErrClosed) {
		t.Errorf("Subscribe after Close = %v, want ErrClosed", err)
	}
}

// Controller.Read is Request with FlagRead set and no body.
func TestController_Read_EmptyBody(t *testing.T) {
	addr, _, cleanup := startServer(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()

	resp, err := c.Read(ctx, testAddr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(resp.Body) != 0 {
		t.Errorf("Body = %q, want empty", resp.Body)
	}
}
