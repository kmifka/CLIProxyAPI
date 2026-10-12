package pluginhost

import (
	"context"
	"errors"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"io"
	"net/http"
	"time"
)

func (a *rpcPluginAdapter) InferenceRoutes(ctx context.Context) ([]pluginapi.InferenceRoute, error) {
	return callPlugin[[]pluginapi.InferenceRoute](ctx, a.client, "inference.register", struct{}{})
}

// Each C ABI call transfers at most one32KiB frame. No whole request/response JSON.
func (a *rpcPluginAdapter) ServeInference(ctx context.Context, r pluginapi.InferenceRequest, body io.Reader, w http.ResponseWriter) error {
	f, err := callPlugin[pluginapi.InferenceFrame](ctx, a.client, "inference.open", r)
	if err != nil {
		return err
	}
	if f.ID == "" {
		return errors.New("missing inference stream")
	}
	id := f.ID
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = callPlugin[pluginapi.InferenceFrame](c, a.client, "inference.cancel", pluginapi.InferenceFrame{ID: id})
	}()
	buf := make([]byte, pluginapi.InferenceChunkBytes)
	var total int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, e := body.Read(buf)
		total += int64(n)
		if total > pluginapi.InferenceMaxBody {
			return errors.New("inference body limit")
		}
		if n > 0 {
			_, err = callPlugin[pluginapi.InferenceFrame](ctx, a.client, "inference.write", pluginapi.InferenceFrame{ID: id, Data: buf[:n]})
			if err != nil {
				return err
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	// Upload ownership transfers before provider execution/rewrite can allocate
	// a second plugin spool. The embedding host retains the budget receipt.
	if closer, ok := body.(io.Closer); ok {
		if err = closer.Close(); err != nil {
			return err
		}
	}
	_, err = callPlugin[pluginapi.InferenceFrame](ctx, a.client, "inference.end", pluginapi.InferenceFrame{ID: id})
	if err != nil {
		return err
	}
	published := false
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		f, err = callPlugin[pluginapi.InferenceFrame](ctx, a.client, "inference.read", pluginapi.InferenceFrame{ID: id})
		if err != nil {
			return err
		}
		if len(f.Data) > pluginapi.InferenceChunkBytes {
			return errors.New("oversized inference frame")
		}
		if published && f.Status == 0 && len(f.Data) == 0 && !f.EOF {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}
		if !published {
			if f.Status == 0 && len(f.Data) == 0 && !f.EOF {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
				continue
			}
			if f.Status < 200 || f.Status > 599 {
				return errors.New("invalid inference status")
			}
			for k, v := range f.Headers {
				switch http.CanonicalHeaderKey(k) {
				case "Content-Type", "Content-Encoding", "Cache-Control", "X-Request-Id":
					w.Header()[k] = v
				}
			}
			w.WriteHeader(f.Status)
			published = true
		}
		if len(f.Data) > 0 {
			if _, err = w.Write(f.Data); err != nil {
				return err
			}
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		}
		if f.EOF {
			return nil
		}
	}
}
