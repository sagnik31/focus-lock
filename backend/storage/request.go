package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
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

// writeJSONAtomic writes v as JSON via temp-file + rename.
func writeJSONAtomic(path string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --- UI side ---

// SubmitRequest writes a request for the Ghost and blocks until the Ghost
// writes a matching result or the timeout elapses. It returns the policy
// error verbatim if the Ghost refused the operation.
func (s *Store) SubmitRequest(op Operation, timeout time.Duration) error {
	dir := filepath.Dir(s.filePath)
	req := Request{ID: newRequestID(), Op: op}

	// Clear any stale result for a previous request before submitting.
	_ = os.Remove(resultPath(dir))

	if err := writeJSONAtomic(requestPath(dir), req); err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res, ok := readResult(dir)
		if ok && res.ID == req.ID {
			_ = os.Remove(resultPath(dir))
			if !res.OK {
				return errorString(res.Error)
			}
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errorString("enforcement process did not respond; is Focus Lock running?")
}

// --- Ghost side ---

// TakePendingRequest returns a queued request (if any) and removes the request
// file so it is processed once. ok is false when no request is waiting.
func TakePendingRequest(dir string) (Request, bool) {
	data, err := os.ReadFile(requestPath(dir))
	if err != nil {
		return Request{}, false
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		// Malformed request: drop it so it does not wedge the queue.
		_ = os.Remove(requestPath(dir))
		return Request{}, false
	}
	_ = os.Remove(requestPath(dir))
	return req, true
}

// WriteResult records the Ghost's verdict for the UI to read back.
func WriteResult(dir string, res Result) {
	_ = writeJSONAtomic(resultPath(dir), res)
}

func readResult(dir string) (Result, bool) {
	data, err := os.ReadFile(resultPath(dir))
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
