// zone simulates a zone controller that publishes periodic status updates.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/mock"
)

func main() {
	ctrl := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		fmt.Printf("[zone front-left] received cmd id=%d type=%d payload=%s\n",
			cmd.ID, cmd.Type, string(cmd.Payload))
		return &rcp.Response{
			CommandID: cmd.ID,
			Zone:      rcp.ZoneFrontLeft,
			Status:    rcp.StatusOK,
			Payload:   []byte(`{"ack":true}`),
		}
	})
	defer ctrl.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}

	go func() {
		for s := range ch {
			fmt.Printf("[zone front-left] status seq=%d healthy=%v payload=%s\n",
				s.Seq, s.Healthy, string(s.Payload))
		}
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctrl.Publish([]byte(`{"temp_c":22,"voltage_v":12.1}`))
	}
}
