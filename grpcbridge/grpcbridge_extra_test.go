package grpcbridge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/grpcbridge"
)

func TestController_Zone(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneCentral, nil)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneCentral, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer func() { _ = c.Close() }()
	if c.Zone() != rcp.ZoneCentral {
		t.Errorf("Zone() = %v, want ZoneCentral", c.Zone())
	}
}

func TestController_Subscribe_AfterClose(t *testing.T) {
	addr, cleanup := startServer(t, rcp.ZoneFrontLeft, nil)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := grpcbridge.NewController(ctx, rcp.ZoneFrontLeft, addr)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := c.Subscribe(ctx); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Subscribe after Close = %v, want ErrClosed", err)
	}
}
