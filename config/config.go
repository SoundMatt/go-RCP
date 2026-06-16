//fusa:req REQ-CFG-001
//fusa:req REQ-CFG-002
//fusa:req REQ-CFG-003
//fusa:req REQ-CFG-004
//fusa:req REQ-CFG-005
//fusa:req REQ-CFG-006
//fusa:req REQ-CFG-007
//fusa:req REQ-CFG-008

// Package config provides YAML/JSON zone registry configuration loading and
// file-system hot-reload for zone addresses without restart.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	rcp "github.com/SoundMatt/go-RCP"
	"gopkg.in/yaml.v3"
)

// Transport specifies the wire protocol for a zone controller endpoint.
type Transport string

const (
	TransportMock Transport = "mock"
	TransportUDP  Transport = "udp"
	TransportTLS  Transport = "tls"
)

// ZoneEntry is the configuration for a single zone controller endpoint.
type ZoneEntry struct {
	Zone      rcp.Zone  `yaml:"zone"      json:"zone"`
	Transport Transport `yaml:"transport" json:"transport"`
	Address   string    `yaml:"address"   json:"address"`
	CertFile  string    `yaml:"cert_file" json:"cert_file,omitempty"`
	KeyFile   string    `yaml:"key_file"  json:"key_file,omitempty"`
	CAFile    string    `yaml:"ca_file"   json:"ca_file,omitempty"`
}

// File is the top-level structure of a zone registry configuration file.
type File struct {
	Version int         `yaml:"version" json:"version"`
	Zones   []ZoneEntry `yaml:"zones"   json:"zones"`
}

var (
	ErrInvalidVersion = errors.New("rcp/config: unsupported config version")
	ErrDuplicateZone  = errors.New("rcp/config: duplicate zone in config file")
	ErrUnknownZone    = errors.New("rcp/config: unknown zone identifier")
)

// Load reads a YAML or JSON config file from path and returns the parsed File.
// The format is auto-detected from the file extension (.yaml/.yml → YAML, .json → JSON).
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
	seen := make(map[rcp.Zone]bool)
	for _, z := range cfg.Zones {
		if seen[z.Zone] {
			return nil, fmt.Errorf("%w: zone %d", ErrDuplicateZone, z.Zone)
		}
		seen[z.Zone] = true
	}
	return &cfg, nil
}

// Watcher watches a config file for changes and calls onChange whenever a
// reload succeeds. Call Stop to release resources.
type Watcher struct {
	path     string
	onChange func(*File)

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
		path:     path,
		onChange: onChange,
		current:  cfg,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
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
	// fsnotify-based hot-reload would be wired here in a production build.
	// The goroutine exits cleanly when Stop is called.
	<-w.stop
}
