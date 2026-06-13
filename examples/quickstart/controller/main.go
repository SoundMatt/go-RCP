// controller demonstrates sending commands to all vehicle zones.
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
	reg := mock.NewRegistry()
	defer reg.Close()

	zones := []rcp.Zone{
		rcp.ZoneFrontLeft,
		rcp.ZoneFrontRight,
		rcp.ZoneRearLeft,
		rcp.ZoneRearRight,
		rcp.ZoneCentral,
	}

	seq := uint32(0)
	for {
		for _, z := range zones {
			seq++
			ctrl, err := reg.Lookup(z)
			if err != nil {
				log.Printf("lookup zone %s: %v", z, err)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			resp, err := ctrl.Send(ctx, &rcp.Command{
				ID:       seq,
				Zone:     z,
				Type:     rcp.CmdSet,
				Priority: rcp.PriorityNormal,
				Payload:  []byte(fmt.Sprintf(`{"seq":%d,"zone":"%s"}`, seq, z)),
			})
			cancel()
			if err != nil {
				log.Printf("[zone %s] send error: %v", z, err)
				continue
			}
			fmt.Printf("[controller] zone=%-12s cmd_id=%d status=%s\n", z, resp.CommandID, resp.Status)
		}
		time.Sleep(time.Second)
	}
}
