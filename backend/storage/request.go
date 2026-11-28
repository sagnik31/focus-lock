package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"focus-lock/backend/userfs"
)

// Request/result files implement the one-way control channel from the
// non-elevated UI to the elevated Ghost. In strong mode the UI cannot sign
// the config, so instead of writing it directly the UI drops a Request here;
// the Ghost (which holds the HMAC secret) validates it against the operation
// policy, applies and signs it, then writes a Result.
//
// The files live in the user-writable config dir on purpose: their contents
// are untrusted. Security comes entirely from the Ghost re-validating every
// operation with ApplyOperation — never from who could write the file.

// Request is one UI-submitted operation, tagged with a unique ID.
type Request struct {
	ID string    `json:"id"`
	Op Operation `json:"op"`
	// Expires (Unix ms) is when the UI stops waiting. A request left over from
	// then is dropped: the user was told that it failed. 0 means never.
	Expires int64 `json:"expires,omitempty"`
}

// Result is the Ghost's verdict on a Request.
type Result struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func requestPath(dir string) string { return filepath.Join(dir, "request.json") }
func resultPath(dir string) string  { return filepath.Join(dir, "result.json") }

// newRequestID returns a process-unique-ish id (pid + nanos).
func newRequestID() string {
	return strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// writeJSONAtomic writes v as JSON via temp-file + rename, with the user's
// rights (see userio.go): the elevated enforcer writes results here.
func writeJSONAtomic(path string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeUser(path, data)
}

// --- UI side ---

// SubmitRequest writes a request for the Ghost and blocks until the Ghost
// writes a matching result or the timeout elapses. It returns the policy
// error verbatim if the Ghost refused the operation.
//
// Requests are serialised per process: the channel is a single request.json
// slot, so two concurrent submissions used to overwrite each other's request
// (and delete each other's result), losing one change and timing out.
func (s *Store) SubmitRequest(op Operation, timeout time.Duration) error {
	s.reqMu.Lock()
	defer s.reqMu.Unlock()

	dir := filepath.Dir(s.filePath)
	deadline := time.Now().Add(timeout)
	req := Request{ID: newRequestID(), Op: op, Expires: deadline.UnixMilli()}

	// Clear any stale result for a previous request before submitting.
	removeUser(resultPath(dir))

	if err := writeJSONAtomic(requestPath(dir), req); err != nil {
		return err
	}

	if err, ok := awaitResult(dir, req.ID, deadline); ok {
		return err
	}
	// Taking the request back succeeds only if the Ghost hasn't claimed it.
	if userfs.Do(func() error { return os.Remove(requestPath(dir)) }) == nil {
		return errorString("enforcement process did not respond; is Focus Lock running?")
	}
	// It has, so the verdict is on its way.
	if err, ok := awaitResult(dir, req.ID, time.Now().Add(timeout)); ok {
		return err
	}
	return errorString("the enforcement process did not confirm the change; check whether it was applied before trying again")
}

// awaitResult waits for the verdict on request id; ok is false at the deadline.
func awaitResult(dir, id string, deadline time.Time) (verdict error, ok bool) {
	for time.Now().Before(deadline) {
		if res, found := readResult(dir); found && res.ID == id {
			removeUser(resultPath(dir))
			if !res.OK {
				return errorString(res.Error), true
			}
			return nil, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, false
}

// --- Ghost side ---

// TakePendingRequest returns a queued request (if any) and removes the request
// file so it is processed once. ok is false when no request is waiting.
func TakePendingRequest(dir string) (Request, bool) {
	// Renaming claims the request: the UI can no longer take it back.
	claimed := requestPath(dir) + ".taken"
	var data []byte
	err := userfs.Do(func() error {
		if err := os.Rename(requestPath(dir), claimed); err != nil {
			return err
		}
		defer os.Remove(claimed)
		var err error
		data, err = os.ReadFile(claimed)
		return err
	})
	if err != nil {
		return Request{}, false
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return Request{}, false // malformed: dropped
	}
	if req.Expires != 0 && time.Now().UnixMilli() > req.Expires {
		return Request{}, false // expired: dropped
	}
	return req, true
}

// WriteResult records the Ghost's verdict for the UI to read back.
func WriteResult(dir string, res Result) {
	_ = writeJSONAtomic(resultPath(dir), res)
}

func readResult(dir string) (Result, bool) {
	data, err := readUser(resultPath(dir))
	if err != nil {
		return Result{}, false
	}
	var res Result
	if err := json.Unmarshal(data, &res); err != nil {
		return Result{}, false
	}
	return res, true
}

// RequestDir returns the directory where request/result files live.
func (s *Store) RequestDir() string { return filepath.Dir(s.filePath) }

// errorString is a tiny error type so callers get the Ghost's message.
type errorString string

func (e errorString) Error() string { return string(e) }
