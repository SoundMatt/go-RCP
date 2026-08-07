// zone runs one real RC Server, over UDP/IP, with a single demo endpoint
// that logs each request it receives and answers with a fixed
// acknowledgement payload. Run examples/quickstart/controller (a separate
// process) against it to see a real request/response round trip — set
// RCP_DEMO_ADDR on the controller if this process isn't reachable at
// 127.0.0.1:7657 (e.g. the two run in separate Docker containers; see
// docker/docker-compose.yml).
package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/mock"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

const (
	demoAddr avtp.ByteBusID = 1
	// listenAddr binds every interface, not just loopback, so this process
	// is reachable from another container on the same bridge network (see
	// docker/docker-compose.yml) as well as from a peer process on the same
	// host via 127.0.0.1.
	listenAddr = "0.0.0.0:7657"
)

func main() {
	stream := avtp.NewStreamID([6]byte{0x02, 0x71, 0x75, 0x69, 0x63, 0x6b}, 1) // "quick" in the low 5 MAC bytes

	router := udp.NewRouter(nil, false) // no EP0 handler: this demo skips server/discovery
	ep := mock.NewEndpoint(demoAddr, func(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
		fmt.Printf("[endpoint %d] request from %s: read=%v write=%v body=%s\n",
			demoAddr, requester, req.Control.Has(acf.FlagRead), req.Control.Has(acf.FlagWrite), string(req.Body))
		return acf.Message{
			Kind:           req.Kind,
			ByteBusID:      req.ByteBusID,
			TransactionNum: req.TransactionNum,
			Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
			Body:           []byte(`{"ack":true}`),
		}, nil
	})
	if err := router.Register(demoAddr, ep); err != nil {
		log.Fatalf("register endpoint: %v", err)
	}

	srv, err := udp.NewServer(stream, listenAddr, router)
	if err != nil {
		log.Fatalf("new server: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	fmt.Printf("endpoint %d listening on %s — press Ctrl+C to stop\n", demoAddr, srv.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
