package mock_test

import (
	"context"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/regmap"
)

// BenchmarkClient_Request_RoundTrip measures the in-process Client -> Router
// -> Endpoint -> Client round trip this package's Fixture wires up, the
// TC18-model replacement for the retired BenchmarkSend_RoundTrip.
func BenchmarkClient_Request_RoundTrip(b *testing.B) {
	fx, err := mock.NewFixture(testStream(), false)
	if err != nil {
		b.Fatalf("NewFixture: %v", err)
	}
	defer func() { _ = fx.Close() }()
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		b.Fatalf("AddEndpoint: %v", err)
	}
	if err := fx.Router.Register(1, mock.NewEndpoint(1, nil)); err != nil {
		b.Fatalf("Register: %v", err)
	}

	ctx := context.Background()
	body := []byte("hello")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := fx.Root.Write(ctx, 1, body); err != nil {
			b.Fatalf("Write: %v", err)
		}
	}
}

// BenchmarkClient_Request_Concurrent is BenchmarkClient_Request_RoundTrip
// under b.RunParallel, the replacement for the retired
// BenchmarkSend_Concurrent.
func BenchmarkClient_Request_Concurrent(b *testing.B) {
	fx, err := mock.NewFixture(testStream(), false)
	if err != nil {
		b.Fatalf("NewFixture: %v", err)
	}
	defer func() { _ = fx.Close() }()
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		b.Fatalf("AddEndpoint: %v", err)
	}
	if err := fx.Router.Register(1, mock.NewEndpoint(1, nil)); err != nil {
		b.Fatalf("Register: %v", err)
	}

	ctx := context.Background()
	body := []byte("hello")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := fx.Root.Write(ctx, 1, body); err != nil {
				b.Fatalf("Write: %v", err)
			}
		}
	})
}

// BenchmarkEndpoint_HandleRequest measures a bare Endpoint.HandleRequest
// call with no Router/Client in the path, isolating dispatch overhead from
// this package's own in-process "wire."
func BenchmarkEndpoint_HandleRequest(b *testing.B) {
	ep := mock.NewEndpoint(1, nil)
	stream := testStream()
	req := acf.Message{ByteBusID: 1, Control: acf.FlagWrite, Body: []byte("hello")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ep.HandleRequest(stream, req); err != nil {
			b.Fatalf("HandleRequest: %v", err)
		}
	}
}

// BenchmarkClientRegistry_Lookup is the ClientRegistry replacement for the
// retired BenchmarkRegistry_Lookup.
func BenchmarkClientRegistry_Lookup(b *testing.B) {
	fx, err := mock.NewFixture(testStream(), false)
	if err != nil {
		b.Fatalf("NewFixture: %v", err)
	}
	defer func() { _ = fx.Close() }()

	reg := mock.NewClientRegistry()
	defer func() { _ = reg.Close() }()
	if err := reg.Register("root", fx.Root); err != nil {
		b.Fatalf("Register: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := reg.Lookup("root"); err != nil {
			b.Fatalf("Lookup: %v", err)
		}
	}
}
