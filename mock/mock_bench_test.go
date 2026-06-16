package mock_test

import (
	"context"
	"fmt"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

var payloadSizes = []struct {
	name string
	size int
}{
	{"1B", 1},
	{"64B", 64},
	{"1KB", 1024},
	{"16KB", 16 * 1024},
	{"64KB", 64 * 1024},
}

// BenchmarkSend_RoundTrip measures the full command→response round-trip
// through the mock controller for payloads from 1 B to 64 KiB.
func BenchmarkSend_RoundTrip(b *testing.B) {
	for _, ps := range payloadSizes {
		b.Run(ps.name, func(b *testing.B) {
			ctrl := mock.NewController(rcp.ZoneFrontLeft, nil)
			defer ctrl.Close()
			payload := make([]byte, ps.size)
			b.SetBytes(int64(ps.size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				cmd := &rcp.Command{
					ID:      uint32(i),
					Zone:    rcp.ZoneFrontLeft,
					Type:    rcp.CmdSet,
					Payload: payload,
				}
				_, _ = ctrl.Send(context.Background(), cmd)
			}
		})
	}
}

// BenchmarkSend_Concurrent measures Send throughput under full parallelism.
func BenchmarkSend_Concurrent(b *testing.B) {
	ctrl := mock.NewController(rcp.ZoneCentral, nil)
	defer ctrl.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		cmd := &rcp.Command{Zone: rcp.ZoneCentral, Type: rcp.CmdGet}
		for pb.Next() {
			_, _ = ctrl.Send(context.Background(), cmd)
		}
	})
}

// BenchmarkPublish_FanOut measures Publish→Subscribe delivery to N concurrent subscribers.
func BenchmarkPublish_FanOut(b *testing.B) {
	for _, n := range []int{1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("%dsubs", n), func(b *testing.B) {
			ctrl := mock.NewController(rcp.ZoneCentral, nil)
			defer ctrl.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			channels := make([]<-chan *rcp.Status, n)
			for i := range channels {
				ch, _ := ctrl.Subscribe(ctx)
				channels[i] = ch
			}

			payload := []byte(`{"v":1}`)
			b.SetBytes(int64(len(payload) * n))
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				ctrl.Publish(payload)
				for _, ch := range channels {
					<-ch
				}
			}
		})
	}
}

// BenchmarkRegistry_Lookup measures concurrent hot-path registry lookups.
func BenchmarkRegistry_Lookup(b *testing.B) {
	reg := mock.NewRegistry()
	defer reg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = reg.Lookup(rcp.ZoneFrontLeft)
		}
	})
}
