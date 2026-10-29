package confi

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"sync"
	"time"
)

// Watcher monitors configuration files for changes and reloads the target struct.
type Watcher struct {
	parser   *Parser
	target   interface{}
	initial  reflect.Value
	onChange func(error)
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	mu       sync.RWMutex
}

type fileSnapshot struct {
	exists  bool
	modTime time.Time
	size    int64
	content []byte
}

// Watch parses target initially and starts a background goroutine that polls configured files
// for modifications, re-parsing target and invoking onChange whenever a file changes.
func Watch(target interface{}, onChange func(error), opts ...Option) (*Watcher, error) {
	p := NewParser("", opts...)
	return p.Watch(target, onChange)
}

// Watch parses target initially and starts a background goroutine that polls configured files
// for modifications using this Parser's settings.
func (p *Parser) Watch(target interface{}, onChange func(error)) (*Watcher, error) {
	if _, _, err := extractSchema(target); err != nil {
		return nil, err
	}

	elemVal := reflect.ValueOf(target).Elem()
	initialCopy := deepCopyValue(elemVal)

	if err := p.Parse(target); err != nil {
		return nil, err
	}

	paths := p.collectWatchedFiles(target)
	if len(paths) == 0 {
		return nil, &ParseError{Message: "no configuration files found to watch"}
	}

	interval := p.watchInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	w := &Watcher{
		parser:   p,
		target:   target,
		initial:  initialCopy,
		onChange: onChange,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}

	snapshots := takeSnapshots(paths, nil)

	go w.loop(paths, snapshots)
	return w, nil
}

// RLock acquires a read lock on the watched configuration struct.
func (w *Watcher) RLock() {
	w.mu.RLock()
}

// RUnlock releases the read lock on the watched configuration struct.
func (w *Watcher) RUnlock() {
	w.mu.RUnlock()
}

// Stop stops the file watcher and waits for the polling loop to exit.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	<-w.doneCh
}

// Close stops the file watcher and implements io.Closer.
func (w *Watcher) Close() error {
	w.Stop()
	return nil
}

func (w *Watcher) loop(paths []string, snapshots map[string]fileSnapshot) {
	defer close(w.doneCh)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.mu.RLock()
			currentPaths := w.parser.collectWatchedFiles(w.target)
			w.mu.RUnlock()
			if len(currentPaths) > 0 {
				paths = currentPaths
			}
			nextSnaps := takeSnapshots(paths, snapshots)
			if !snapshotsChanged(snapshots, nextSnaps) {
				continue
			}
			snapshots = nextSnaps

			elemType := reflect.ValueOf(w.target).Elem().Type()
			freshPtr := reflect.New(elemType)
			freshPtr.Elem().Set(deepCopyValue(w.initial))
			err := w.parser.Parse(freshPtr.Interface())
			if err == nil {
				w.mu.Lock()
				reflect.ValueOf(w.target).Elem().Set(freshPtr.Elem())
				w.mu.Unlock()
			}
			if w.onChange != nil {
				go w.onChange(err)
			}
		}
	}
}

func (p *Parser) collectWatchedFiles(target interface{}) []string {
	var paths []string
	addPath := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range paths {
			if existing == path {
				return
			}
		}
		paths = append(paths, path)
	}

	for _, src := range p.configSources {
		addPath(src.path)
	}

	schema, rootVal, err := extractSchema(target)
	if err == nil && schema.configFileField != nil {
		if fv, ok := schema.configFileField.fieldValue(rootVal, false); ok && fv.Kind() == reflect.String {
			addPath(fv.String())
		}
	}
	return paths
}

func takeSnapshots(paths []string, prev map[string]fileSnapshot) map[string]fileSnapshot {
	snaps := make(map[string]fileSnapshot, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			snaps[path] = fileSnapshot{exists: false}
			continue
		}
		modTime := info.ModTime()
		size := info.Size()

		if prev != nil {
			if prevSnap, ok := prev[path]; ok && prevSnap.exists && prevSnap.size == size && prevSnap.modTime.Equal(modTime) {
				if time.Since(modTime) >= 500*time.Millisecond {
					snaps[path] = prevSnap
					continue
				}
			}
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			snaps[path] = fileSnapshot{exists: false}
			continue
		}
		snaps[path] = fileSnapshot{
			exists:  true,
			modTime: modTime,
			size:    size,
			content: data,
		}
	}
	return snaps
}

func deepCopyValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	t := v.Type()
	cp := reflect.New(t).Elem()
	cp.Set(v)

	switch t.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			elemCopy := deepCopyValue(v.Elem())
			newPtr := reflect.New(t.Elem())
			newPtr.Elem().Set(elemCopy)
			cp.Set(newPtr)
		}
	case reflect.Slice:
		if !v.IsNil() {
			newSlice := reflect.MakeSlice(t, v.Len(), v.Cap())
			for i := 0; i < v.Len(); i++ {
				newSlice.Index(i).Set(deepCopyValue(v.Index(i)))
			}
			cp.Set(newSlice)
		}
	case reflect.Map:
		if !v.IsNil() {
			newMap := reflect.MakeMapWithSize(t, v.Len())
			for _, k := range v.MapKeys() {
				newMap.SetMapIndex(deepCopyValue(k), deepCopyValue(v.MapIndex(k)))
			}
			cp.Set(newMap)
		}
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if cp.Field(i).CanSet() {
				cp.Field(i).Set(deepCopyValue(v.Field(i)))
			}
		}
	}
	return cp
}

func snapshotsChanged(prev, next map[string]fileSnapshot) bool {
	if len(prev) != len(next) {
		return true
	}
	for k, pSnap := range prev {
		nSnap, ok := next[k]
		if !ok {
			return true
		}
		if pSnap.exists != nSnap.exists || pSnap.size != nSnap.size || !pSnap.modTime.Equal(nSnap.modTime) || !bytes.Equal(pSnap.content, nSnap.content) {
			return true
		}
	}
	return false
}
