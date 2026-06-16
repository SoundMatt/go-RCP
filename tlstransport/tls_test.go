//fusa:test REQ-TLS-001
//fusa:test REQ-TLS-002
//fusa:test REQ-TLS-003
//fusa:test REQ-TLS-004
//fusa:test REQ-TLS-005
//fusa:test REQ-TLS-006
//fusa:test REQ-TLS-007
//fusa:test REQ-TLS-008
//fusa:test REQ-TLS-009
//fusa:test REQ-TLS-010

package tlstransport_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	rcptls "github.com/SoundMatt/go-RCP/tlstransport"
)

// genCerts generates a self-signed CA + server cert + client cert for testing.
func genCerts(t *testing.T) (serverCfg, clientCfg *tls.Config) {
	t.Helper()

	// CA key + cert
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	makeLeaf := func(cn string) tls.Certificate {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
			DNSNames:     []string{cn, "localhost"},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			t.Fatal(err)
		}
		return cert
	}

	serverCert := makeLeaf("server")
	clientCert := makeLeaf("client")

	serverCfg = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	clientCfg = &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   "server",
		MinVersion:   tls.VersionTLS13,
	}
	return serverCfg, clientCfg
}

func newTLSPair(t *testing.T, zone rcp.Zone) (*rcptls.ZoneServer, *rcptls.Controller) {
	t.Helper()
	serverCfg, clientCfg := genCerts(t)

	srv, err := rcptls.NewZoneServer(zone, "127.0.0.1:0", serverCfg)
	if err != nil {
		t.Fatalf("NewZoneServer: %v", err)
	}
	ctrl, err := rcptls.NewController(zone, srv.Addr().String(), clientCfg)
	if err != nil {
		_ = srv.Close()
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() {
		_ = ctrl.Close()
		_ = srv.Close()
	})
	return srv, ctrl
}

// TestTLS_Send_RoundTrip verifies command + response over mTLS (REQ-TLS-001, REQ-TLS-002).
func TestTLS_Send_RoundTrip(t *testing.T) {
	_, ctrl := newTLSPair(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want OK", resp.Status)
	}
}

// TestTLS_Send_CustomHandler verifies server-side handler invocation (REQ-TLS-003).
func TestTLS_Send_CustomHandler(t *testing.T) {
	srv, ctrl := newTLSPair(t, rcp.ZoneFrontRight)
	srv.SetHandler(func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusError}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontRight, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("status = %v, want Error", resp.Status)
	}
}

// TestTLS_Send_PayloadRoundTrip verifies payload survives TLS framing (REQ-TLS-004).
func TestTLS_Send_PayloadRoundTrip(t *testing.T) {
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	srv, ctrl := newTLSPair(t, rcp.ZoneRearLeft)
	srv.SetHandler(func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK, Payload: cmd.Payload}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet, Payload: want})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !bytes.Equal(resp.Payload, want) {
		t.Errorf("payload = %v, want %v", resp.Payload, want)
	}
}

// TestTLS_Send_ZoneMismatch verifies ErrZoneMismatch (REQ-TLS-005).
func TestTLS_Send_ZoneMismatch(t *testing.T) {
	_, ctrl := newTLSPair(t, rcp.ZoneRearRight)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("error = %v, want ErrZoneMismatch", err)
	}
}

// TestTLS_Send_ContextCancelled verifies ErrTimeout on pre-cancelled context (REQ-TLS-006).
func TestTLS_Send_ContextCancelled(t *testing.T) {
	_, ctrl := newTLSPair(t, rcp.ZoneCentral)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneCentral, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

// TestTLS_Send_AfterClose verifies ErrClosed (REQ-TLS-007).
func TestTLS_Send_AfterClose(t *testing.T) {
	_, ctrl := newTLSPair(t, rcp.ZoneFrontLeft)
	_ = ctrl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("error = %v, want ErrClosed", err)
	}
}

// TestTLS_Subscribe_ReceivesStatus verifies Publish → Subscribe fan-out over TLS (REQ-TLS-008).
func TestTLS_Subscribe_ReceivesStatus(t *testing.T) {
	srv, ctrl := newTLSPair(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	srv.Publish([]byte{0x01})

	select {
	case st := <-ch:
		if st.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone = %v, want FrontLeft", st.Zone)
		}
	case <-time.After(time.Second):
		t.Fatal("no Status received within 1s")
	}
}

// TestTLS_Subscribe_ClosedOnContextCancel verifies channel closes on ctx cancel (REQ-TLS-009).
func TestTLS_Subscribe_ClosedOnContextCancel(t *testing.T) {
	_, ctrl := newTLSPair(t, rcp.ZoneRearLeft)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed within 1s")
	}
}

// TestTLS_UntrustedClient verifies that a client without a valid cert is rejected (REQ-TLS-010).
func TestTLS_UntrustedClient(t *testing.T) {
	serverCfg, _ := genCerts(t)
	// Different CA for a bad client — not trusted by the server
	_, badClientCfg := genCerts(t)
	badClientCfg.ServerName = "server"

	srv, err := rcptls.NewZoneServer(rcp.ZoneFrontLeft, "127.0.0.1:0", serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	// Dial with mismatched CA — should fail TLS handshake
	_, err = rcptls.NewController(rcp.ZoneFrontLeft, srv.Addr().String(), badClientCfg)
	if err == nil {
		t.Error("expected TLS handshake to fail for untrusted client, but got nil error")
	}
}

// TestTLS_Registry_DialAndLookup verifies Registry.Dial + Lookup (REQ-TLS-001).
func TestTLS_Registry_DialAndLookup(t *testing.T) {
	serverCfg, clientCfg := genCerts(t)
	srv, err := rcptls.NewZoneServer(rcp.ZoneCentral, "127.0.0.1:0", serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	reg := rcptls.NewRegistry()
	defer func() { _ = reg.Close() }()

	if dialErr := reg.Dial(rcp.ZoneCentral, srv.Addr().String(), clientCfg); dialErr != nil {
		t.Fatalf("Dial: %v", dialErr)
	}

	ctrl, err := reg.Lookup(rcp.ZoneCentral)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneCentral, Type: rcp.CmdNoop})
			if err != nil {
				t.Errorf("concurrent Send: %v", err)
				return
			}
			if resp.Status != rcp.StatusOK {
				t.Errorf("status = %v, want OK", resp.Status)
			}
		}()
	}
	wg.Wait()
}
