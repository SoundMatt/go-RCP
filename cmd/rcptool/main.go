// rcptool is a CLI for interacting with go-RCP zone controllers.
//
// Usage:
//
//	rcptool discover       – list all registered zones
//	rcptool send <zone>    – send a CmdSet to the given zone
//	rcptool monitor        – stream status updates from all zones
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	reg := mock.NewRegistry()
	defer reg.Close()

	switch os.Args[1] {
	case "discover":
		cmdDiscover(reg)
	case "send":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: rcptool send <zone>")
			os.Exit(1)
		}
		cmdSend(reg, os.Args[2])
	case "monitor":
		cmdMonitor(reg)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rcptool <discover|send <zone>|monitor>")
}

func cmdDiscover(reg *mock.Registry) {
	for _, ctrl := range reg.Controllers() {
		fmt.Printf("zone %-12s  controller=%T\n", ctrl.Zone(), ctrl)
	}
}

func cmdSend(reg *mock.Registry, zoneName string) {
	zone := parseZone(zoneName)
	ctrl, err := reg.Lookup(zone)
	if err != nil {
		log.Fatalf("zone %q not found: %v", zoneName, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{
		ID:       1,
		Zone:     zone,
		Type:     rcp.CmdGet,
		Priority: rcp.PriorityNormal,
	})
	if err != nil {
		log.Fatalf("send: %v", err)
	}
	b, _ := json.MarshalIndent(map[string]any{
		"command_id": resp.CommandID,
		"zone":       resp.Zone.String(),
		"status":     resp.Status.String(),
		"payload":    string(resp.Payload),
	}, "", "  ")
	fmt.Println(string(b))
}

func cmdMonitor(reg *mock.Registry) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for _, ctrl := range reg.Controllers() {
		mc, ok := ctrl.(*mock.Controller)
		if !ok {
			continue
		}
		ch, err := mc.Subscribe(ctx)
		if err != nil {
			log.Printf("subscribe zone %s: %v", mc.Zone(), err)
			continue
		}
		go func(z rcp.Zone, ch <-chan *rcp.Status) {
			for s := range ch {
				fmt.Printf("[%s] seq=%d healthy=%v payload=%s\n", z, s.Seq, s.Healthy, string(s.Payload))
			}
		}(mc.Zone(), ch)

		go func(mc *mock.Controller) {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					mc.Publish([]byte(`{"heartbeat":true}`))
				}
			}
		}(mc)
	}

	fmt.Println("monitoring all zones — press Ctrl+C to stop")
	<-ctx.Done()
}

func parseZone(s string) rcp.Zone {
	switch s {
	case "front-left":
		return rcp.ZoneFrontLeft
	case "front-right":
		return rcp.ZoneFrontRight
	case "rear-left":
		return rcp.ZoneRearLeft
	case "rear-right":
		return rcp.ZoneRearRight
	case "central":
		return rcp.ZoneCentral
	default:
		fmt.Fprintf(os.Stderr, "unknown zone %q; valid: front-left, front-right, rear-left, rear-right, central\n", s)
		os.Exit(1)
		return rcp.ZoneUnknown
	}
}
