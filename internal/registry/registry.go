package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/atomicfile"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
)

var ErrConflict = errors.New("configuration changed; reload before saving")
var ErrPersistence = errors.New("cannot persist configuration")

type Document struct {
	Revision string        `json:"revision"`
	Config   config.Config `json:"config"`
}
type Registry struct {
	mu      sync.Mutex
	path    string
	data    []byte
	disk    []byte
	ready   bool
	hadFile bool
	problem string
	wake    chan struct{}
}

func New(path string) *Registry {
	r := &Registry{path: path, wake: make(chan struct{}, 1)}
	r.reload(true)
	return r
}
func canonical(c config.Config) []byte {
	data, _ := json.MarshalIndent(c, "", "  ")
	return append(data, '\n')
}
func revision(data []byte) string         { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func (r *Registry) Wake() <-chan struct{} { return r.wake }
func (r *Registry) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
func (r *Registry) reload(initial bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := os.ReadFile(r.path)
	if initial && os.IsNotExist(err) {
		r.data = canonical(config.Config{})
		r.ready = true
		r.problem = ""
		return
	}
	if os.IsNotExist(err) && r.ready && !r.hadFile {
		return
	}
	if err != nil {
		if os.IsNotExist(err) {
			r.disk = nil
		}
		r.problem = err.Error()
		return
	}
	if r.hadFile && bytes.Equal(data, r.disk) {
		return
	}
	r.disk = append([]byte(nil), data...)
	r.hadFile = true
	c, err := config.Parse(data)
	if err != nil {
		r.problem = err.Error()
		return
	}
	r.data = canonical(c)
	r.ready = true
	r.problem = ""
	r.signal()
}
func (r *Registry) Reload() { r.reload(false) }
func (r *Registry) Snapshot() (Document, bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var c config.Config
	if r.ready {
		c, _ = config.Parse(r.data)
	}
	return Document{Revision: revision(r.data), Config: c}, r.ready, r.problem
}
func (r *Registry) Replace(expected string, data []byte) (Document, error) {
	c, err := config.Parse(data)
	if err != nil {
		return Document{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if expected != revision(r.data) {
		return Document{}, ErrConflict
	}
	// Compare against disk too: external edits must not be silently overwritten.
	current, err := os.ReadFile(r.path)
	if err != nil && !os.IsNotExist(err) {
		return Document{}, fmt.Errorf("%w: %v", ErrPersistence, err)
	}
	if !bytes.Equal(current, r.disk) {
		return Document{}, ErrConflict
	}
	encoded := canonical(c)
	// Preserve the first on-disk document, including legacy comments. Never
	// overwrite this migration backup on subsequent edits.
	if current != nil {
		if _, err := os.Stat(r.path + ".bak"); os.IsNotExist(err) {
			if err := atomicfile.Write(r.path+".bak", current, 0600); err != nil {
				return Document{}, fmt.Errorf("%w: backup: %v", ErrPersistence, err)
			}
		} else if err != nil {
			return Document{}, fmt.Errorf("%w: backup: %v", ErrPersistence, err)
		}
	}
	if err := atomicfile.Write(r.path, encoded, 0600); err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrPersistence, err)
	}
	r.data = encoded
	r.disk = append([]byte(nil), encoded...)
	r.ready = true
	r.hadFile = true
	r.problem = ""
	r.signal()
	return Document{Revision: revision(encoded), Config: c}, nil
}
