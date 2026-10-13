package executor

import (
	"io"
	"net/http"
)

// Match the existing inference ABI's 128MiB allowance. Raw Manager.HttpRequest
// also serves callers outside bounded public handlers; never ReadAll unbounded.
func readHomeUltrafastHTTPBody(req *http.Request, limit int64) ([]byte, error) {
	if req.ContentLength > limit {
		return nil, statusErr{code: http.StatusRequestEntityTooLarge, msg: "Home Codex request body exceeds limit"}
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, statusErr{code: http.StatusRequestEntityTooLarge, msg: "Home Codex request body exceeds limit"}
	}
	return body, nil
}
