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
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxHeader      = 4096
	rescanEvery    = 30 * time.Second // how often to look for entries written by another process
	tempGrace      = time.Hour        // abandoned temp files older than this are removed
	futureSlack    = 5 * time.Minute  // fetched times further ahead than this are implausible
	defaultMaxKeep = 2000
)

// Disk is a persistent store of feed bodies keyed by URL, so content fetched
// in the background (or by a separate `sync` run) is there at start-up and
// reading never waits for the network. One file per URL: a JSON header line,
// then the body. Writes are atomic and private to the user. An in-memory
// index of headers answers "is it cached, and how old" without reading
// bodies, and is refreshed periodically to see other processes' writes.
type Disk struct {
	dir string

	mu      sync.Mutex
	index   map[string]time.Time // URL -> fetched
	scanned time.Time
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
	d := &Disk{dir: dir}
	d.rescanLocked()
	return d, nil
}

func (d *Disk) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(d.dir, hex.EncodeToString(sum[:12])+".feed")
}

func readHeader(path string) (diskHeader, *bufio.Reader, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return diskHeader{}, nil, nil, err
	}
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		f.Close()
		return diskHeader{}, nil, nil, errors.New("not a regular file")
	}
	r := bufio.NewReaderSize(f, maxHeader)
	line, err := r.ReadSlice('\n')
	if err != nil {
		f.Close()
		return diskHeader{}, nil, nil, errors.New("no header line")
	}
	var h diskHeader
	if err := json.Unmarshal(bytes.TrimSpace(line), &h); err != nil || h.URL == "" {
		f.Close()
		return diskHeader{}, nil, nil, errors.New("bad header")
	}
	return h, r, f, nil
}

func headerOnly(path string) (diskHeader, error) {
	h, _, f, err := readHeader(path)
	if err != nil {
		return diskHeader{}, err
	}
	f.Close()
	return h, nil
}

// rescanLocked rebuilds the index from the headers on disk.
func (d *Disk) rescanLocked() {
	d.index = map[string]time.Time{}
	entries, err := os.ReadDir(d.dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".feed") {
				continue
			}
			if h, err := headerOnly(filepath.Join(d.dir, e.Name())); err == nil {
				d.index[h.URL] = time.Unix(h.Fetched, 0).UTC()
			}
		}
	}
	d.scanned = time.Now()
}

func (d *Disk) freshenLocked() {
	if time.Since(d.scanned) > rescanEvery {
		d.rescanLocked()
	}
}

// Rescan rebuilds the index now, to see entries written by another process.
func (d *Disk) Rescan() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rescanLocked()
}

// Put stores body for url as fetched at the given time. It never replaces an
// entry that was fetched more recently, whoever wrote it.
func (d *Disk) Put(url string, body []byte, fetched time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	target := d.path(url)
	if h, err := headerOnly(target); err == nil && h.URL == url && h.Fetched > fetched.Unix() {
		d.index[url] = time.Unix(h.Fetched, 0).UTC()
		return nil
	}
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
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, target); err != nil {
		os.Remove(name)
		return err
	}
	d.index[url] = time.Unix(fetched.Unix(), 0).UTC()
	return nil
}

// Get returns the stored body for url and when it was fetched.
func (d *Disk) Get(url string) ([]byte, time.Time, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, r, f, err := readHeader(d.path(url))
	if err != nil || h.URL != url {
		if f != nil {
			f.Close()
		}
		delete(d.index, url)
		return nil, time.Time{}, false
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil || len(body) > maxBody {
		delete(d.index, url)
		return nil, time.Time{}, false
	}
	fetched := time.Unix(h.Fetched, 0).UTC()
	d.index[url] = fetched
	return body, fetched, true
}

// Fetched reports when url was fetched, from the index, without reading the body.
func (d *Disk) Fetched(url string) (time.Time, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.freshenLocked()
	if t, ok := d.index[url]; ok {
		return t, true
	}
	if h, err := headerOnly(d.path(url)); err == nil && h.URL == url { // written elsewhere since the last scan
		t := time.Unix(h.Fetched, 0).UTC()
		d.index[url] = t
		return t, true
	}
	return time.Time{}, false
}

// Stats counts entries from the index.
func (d *Disk) Stats(_ time.Time) DiskStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.freshenLocked()
	var st DiskStats
	for url, t := range d.index {
		st.Entries++
		if strings.Contains(url, "/comments/") {
			st.Threads++
		} else {
			st.Listings++
		}
		if t.After(st.Newest) {
			st.Newest = t
		}
	}
	return st
}

// Prune removes what should not be kept, with the default entry cap.
func (d *Disk) Prune(now time.Time, maxAge time.Duration) int {
	return d.PruneAt(now, maxAge, defaultMaxKeep)
}

// PruneAt deletes entries fetched more than maxAge before now or implausibly
// far in the future, unreadable entries, temp files abandoned by a crash, and
// then the oldest entries beyond maxEntries. It reports how many files went.
func (d *Disk) PruneAt(now time.Time, maxAge time.Duration, maxEntries int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	removed := 0
	remove := func(p string) {
		if os.Remove(p) == nil {
			removed++
		}
	}
	type kept struct {
		path    string
		fetched time.Time
	}
	var keep []kept
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		p := filepath.Join(d.dir, e.Name())
		switch {
		case e.IsDir():
		case strings.HasSuffix(e.Name(), ".tmp"):
			if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > tempGrace {
				remove(p)
			}
		case strings.HasSuffix(e.Name(), ".feed"):
			h, err := headerOnly(p)
			if err != nil {
				remove(p)
				continue
			}
			fetched := time.Unix(h.Fetched, 0)
			if now.Sub(fetched) > maxAge || fetched.Sub(now) > futureSlack {
				remove(p)
				continue
			}
			keep = append(keep, kept{p, fetched})
		}
	}
	if maxEntries > 0 && len(keep) > maxEntries {
		sort.Slice(keep, func(i, j int) bool { return keep[i].fetched.After(keep[j].fetched) })
		for _, k := range keep[maxEntries:] {
			remove(k.path)
		}
	}
	d.rescanLocked()
	return removed
}
