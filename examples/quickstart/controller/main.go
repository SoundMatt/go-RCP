// controller demonstrates writing to a real RC Server endpoint over UDP/IP.
// Run examples/quickstart/zone first (a separate process) to have something
// listening. Defaults to 127.0.0.1:7657; set RCP_DEMO_ADDR to target a
// zone process reachable at a different host:port (e.g. "zone:7657" when
// running as the docker-compose "controller" service against the "zone"
// service — see docker/docker-compose.yml).
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

const (
	demoAddr        avtp.ByteBusID = 1
	defaultDemoAddr                = "127.0.0.1:7657"
)

func main() {
	target := os.Getenv("RCP_DEMO_ADDR")
	if target == "" {
		target = defaultDemoAddr
	}
	serverAddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		log.Fatalf("resolve %s: %v", target, err)
	}

	clientStream := avtp.NewStreamID([6]byte{0x02, 0x63, 0x74, 0x72, 0x6c, 0x72}, 1) // "ctrlr" in the low 5 MAC bytes
	ctrl, err := udp.NewController(clientStream, serverAddr)
	if err != nil {
		log.Fatalf("new controller: %v", err)
	}
	defer ctrl.Close() //nolint:errcheck

	seq := uint32(0)
	for {
		seq++
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		resp, err := ctrl.Write(ctx, demoAddr, []byte(fmt.Sprintf(`{"seq":%d}`, seq)))
		cancel()
		if err != nil {
			log.Printf("[endpoint %d] write error: %v", demoAddr, err)
		} else {
			fmt.Printf("[controller] byte_bus_id=%d seq=%d error=%v payload=%s\n",
				demoAddr, seq, resp.Control.Has(acf.FlagError), string(resp.Body))
		}
		time.Sleep(time.Second)
	}
}
