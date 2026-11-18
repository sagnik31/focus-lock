package storage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Several UI operations submitted at once (e.g. two preset clicks) must all
// reach the Ghost and get their own result.
func TestSubmitRequestConcurrent(t *testing.T) {
	dir := t.TempDir()
	ui := &Store{filePath: filepath.Join(dir, "config.json")} // strong-mode UI: cannot sign

	// Fake Ghost: drain one request per tick and approve it.
	stop := make(chan struct{})
	seen := make(chan string, 16)
	go func() {
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if req, ok := TakePendingRequest(dir); ok {
					seen <- req.Op.URL
					WriteResult(dir, Result{ID: req.ID, OK: true})
				}
			}
		}
	}()
	defer close(stop)

	sites := []string{"a.com", "b.com", "c.com", "d.com"}
	var wg sync.WaitGroup
	errs := make(chan error, len(sites))
	for _, s := range sites {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			errs <- ui.SubmitRequest(Operation{Type: OpAddSite, URL: s}, 3*time.Second)
		}(s)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("SubmitRequest: %v", err)
		}
	}
	close(seen)
	got := map[string]bool{}
	for u := range seen {
		got[u] = true
	}
	for _, s := range sites {
		if !got[s] {
			t.Errorf("request for %s never reached the Ghost", s)
		}
	}
}
