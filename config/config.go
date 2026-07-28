//fusa:req REQ-CFG-001
//fusa:req REQ-CFG-002
//fusa:req REQ-CFG-003
//fusa:req REQ-CFG-004
//fusa:req REQ-CFG-005
//fusa:req REQ-CFG-006
//fusa:req REQ-CFG-007
//fusa:req REQ-CFG-008

// Package config provides YAML/JSON server configuration loading and
// file-system hot-reload without restart, for the OPEN Alliance TC18 Remote
// Control Protocol (RCP), as described by the "OPEN Alliance TC18 Remote
// Control Protocol Specification v0.5.1_RC".
//
// This is ROADMAP.md Milestone 55 (v0.68.0)'s ADAPT-flagged rebuild: per
// Phase 17's disposition table, "YAML/JSON config loading is reusable; the
// schema moves from a zone registry to server/stream/register-map
// configuration." The retired File described a flat zone registry (zone ID,
// transport, address, certs); this package's File instead describes, per
// server: its dial transport and address, its own avtp.StreamID identity,
// and the declared endpoint topology (address + regmap.EndpointType pairs)
// a caller would otherwise have to build up with repeated
// server.Server.AddEndpoint calls by hand. This is a deliberately scoped
// subset of "register-map configuration" — the declared topology, not the
// full binary-encoded regmap.RegisterMap wire format (pin mapping, stream
// limits, per-endpoint functional blocks), which is a server's own runtime
// configuration state, not something this package duplicates in text form.
package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"gopkg.in/yaml.v3"
)

// Transport specifies the wire protocol for a server dial endpoint.
type Transport string

const (
	TransportMock Transport = "mock"
	TransportUDP  Transport = "udp"
	TransportTLS  Transport = "tls"
)

// EndpointEntry declares one endpoint a server presents: its address and
// functional type. This mirrors what a caller would otherwise establish via
// repeated server.Server.AddEndpoint calls.
type EndpointEntry struct {
	Address avtp.ByteBusID      `yaml:"address" json:"address"`
	Type    regmap.EndpointType `yaml:"type"    json:"type"`
}

// ServerEntry is the configuration for a single RC Server dial endpoint.
type ServerEntry struct {
	Key       string          `yaml:"key"        json:"key"`
	StreamID  string          `yaml:"stream_id"  json:"stream_id"` // 16 hex chars (8 bytes)
	Transport Transport       `yaml:"transport"  json:"transport"`
	Address   string          `yaml:"address"    json:"address"`
	Endpoints []EndpointEntry `yaml:"endpoints"  json:"endpoints,omitempty"`
	CertFile  string          `yaml:"cert_file"  json:"cert_file,omitempty"`
	KeyFile   string          `yaml:"key_file"   json:"key_file,omitempty"`
	CAFile    string          `yaml:"ca_file"    json:"ca_file,omitempty"`
}

// DecodeStreamID parses e's StreamID field (16 hex characters) into an
// avtp.StreamID.
func (e ServerEntry) DecodeStreamID() (avtp.StreamID, error) {
	b, err := hex.DecodeString(e.StreamID)
	if err != nil || len(b) != len(avtp.StreamID{}) {
		return avtp.StreamID{}, fmt.Errorf("%w: server %s: stream_id %q", ErrInvalidStreamID, e.Key, e.StreamID)
	}
	var id avtp.StreamID
	copy(id[:], b)
	return id, nil
}

// File is the top-level structure of a server configuration file.
type File struct {
	Version int           `yaml:"version" json:"version"`
	Servers []ServerEntry `yaml:"servers" json:"servers"`
}

var (
	ErrInvalidVersion   = errors.New("rcp/config: unsupported config version")
	ErrDuplicateServer  = errors.New("rcp/config: duplicate server key in config file")
	ErrInvalidStreamID  = errors.New("rcp/config: invalid stream_id")
	ErrDuplicateAddress = errors.New("rcp/config: duplicate endpoint address within a server")
)

// Load reads a YAML or JSON config file from path and returns the parsed
// File. The format is auto-detected from the file extension
// (.yaml/.yml -> YAML, .json -> JSON).
func Load(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("rcp/config: open %s: %w", path, err)
	}
	defer f.Close()
	return Decode(f, filepath.Ext(path))
}

// Decode parses a config from r. ext should be ".yaml", ".yml", or ".json".
func Decode(r io.Reader, ext string) (*File, error) {
	var cfg File
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.NewDecoder(r).Decode(&cfg); err != nil {
			return nil, fmt.Errorf("rcp/config: yaml decode: %w", err)
		}
	case ".json":
		if err := json.NewDecoder(r).Decode(&cfg); err != nil {
			return nil, fmt.Errorf("rcp/config: json decode: %w", err)
		}
	default:
		return nil, fmt.Errorf("rcp/config: unsupported extension %q (use .yaml/.yml or .json)", ext)
	}
	if cfg.Version != 1 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidVersion, cfg.Version)
	}
	seenKeys := make(map[string]bool)
	for _, s := range cfg.Servers {
		if seenKeys[s.Key] {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateServer, s.Key)
		}
		seenKeys[s.Key] = true
		if _, err := s.DecodeStreamID(); err != nil {
			return nil, err
		}
		seenAddrs := make(map[avtp.ByteBusID]bool)
		for _, e := range s.Endpoints {
			if seenAddrs[e.Address] {
				return nil, fmt.Errorf("%w: server %s address %d", ErrDuplicateAddress, s.Key, e.Address)
			}
			seenAddrs[e.Address] = true
		}
	}
	return &cfg, nil
}

// Watcher watches a config file for changes and calls onChange whenever a
// reload succeeds. Call Stop to release resources.
type Watcher struct {
	path         string
	onChange     func(*File)
	pollInterval time.Duration

	mu      sync.RWMutex
	current *File
	stop    chan struct{}
	done    chan struct{}
}

// Watch starts watching path. onChange is called once immediately with the
// initial config, then again on each successful reload.
func Watch(path string, onChange func(*File)) (*Watcher, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		path:         path,
		onChange:     onChange,
		current:      cfg,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		pollInterval: 250 * time.Millisecond,
	}
	onChange(cfg)
	go w.run()
	return w, nil
}

// Current returns the most recently loaded config.
func (w *Watcher) Current() *File {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.current
}

// Reload forces an immediate reload of the config file.
// onChange is called if the reload succeeds.
func (w *Watcher) Reload() error {
	cfg, err := Load(w.path)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.current = cfg
	w.mu.Unlock()
	w.onChange(cfg)
	return nil
}

// Stop terminates the background watcher goroutine.
func (w *Watcher) Stop() {
	close(w.stop)
	<-w.done
}

func (w *Watcher) run() {
	defer close(w.done)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	var lastMod time.Time
	if info, err := os.Stat(w.path); err == nil {
		lastMod = info.ModTime()
	}

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				continue
			}
			if t := info.ModTime(); t != lastMod {
				lastMod = t
				if cfg, err := Load(w.path); err == nil {
					w.mu.Lock()
					w.current = cfg
					w.mu.Unlock()
					w.onChange(cfg)
				}
			}
		}
	}
}
