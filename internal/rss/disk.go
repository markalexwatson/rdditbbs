package rss

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Disk is a persistent store of feed bodies keyed by URL, so content fetched
// in the background (or by a separate `sync` run) is there at start-up and
// reading never waits for the network. One file per URL: a JSON header line,
// then the body. Writes are atomic and private to the user.
type Disk struct {
	dir string
	mu  sync.Mutex
}

type diskHeader struct {
	URL     string `json:"url"`
	Fetched int64  `json:"fetched"` // Unix seconds
}

// DiskStats summarises the store for the UI.
type DiskStats struct {
	Entries, Listings, Threads int
	Newest                     time.Time
}

// NewDisk opens (creating if needed) a store in dir.
func NewDisk(dir string) (*Disk, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	return &Disk{dir: dir}, nil
}

func (d *Disk) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(d.dir, hex.EncodeToString(sum[:12])+".feed")
}

// Put stores body for url as fetched at the given time.
func (d *Disk) Put(url string, body []byte, fetched time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	head, err := json.Marshal(diskHeader{URL: url, Fetched: fetched.Unix()})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d.dir, ".put-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error { tmp.Close(); os.Remove(name); return err }
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(append(head, '\n')); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(body); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, d.path(url)); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func readEntry(path string) (diskHeader, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return diskHeader{}, nil, err
	}
	defer f.Close()
	r := bufio.NewReader(io.LimitReader(f, maxBody+4096))
	line, err := r.ReadBytes('\n')
	if err != nil {
		return diskHeader{}, nil, errors.New("no header line")
	}
	var h diskHeader
	if err := json.Unmarshal(bytes.TrimSpace(line), &h); err != nil || h.URL == "" {
		return diskHeader{}, nil, errors.New("bad header")
	}
	body, err := io.ReadAll(r)
	return h, body, err
}

// Get returns the stored body for url and when it was fetched.
func (d *Disk) Get(url string) ([]byte, time.Time, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, body, err := readEntry(d.path(url))
	if err != nil || h.URL != url {
		return nil, time.Time{}, false
	}
	return body, time.Unix(h.Fetched, 0).UTC(), true
}

// each calls fn for every readable entry.
func (d *Disk) each(fn func(path string, h diskHeader)) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".feed") {
			continue
		}
		p := filepath.Join(d.dir, e.Name())
		if h, _, err := readEntry(p); err == nil {
			fn(p, h)
		}
	}
}

// Stats counts entries; now is accepted for symmetry with Prune and future use.
func (d *Disk) Stats(_ time.Time) DiskStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	var st DiskStats
	d.each(func(_ string, h diskHeader) {
		st.Entries++
		if strings.Contains(h.URL, "/comments/") {
			st.Threads++
		} else {
			st.Listings++
		}
		if t := time.Unix(h.Fetched, 0).UTC(); t.After(st.Newest) {
			st.Newest = t
		}
	})
	return st
}

// Prune deletes entries fetched more than maxAge before now and reports how many.
func (d *Disk) Prune(now time.Time, maxAge time.Duration) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	removed := 0
	d.each(func(path string, h diskHeader) {
		if now.Sub(time.Unix(h.Fetched, 0)) > maxAge {
			if os.Remove(path) == nil {
				removed++
			}
		}
	})
	return removed
}
