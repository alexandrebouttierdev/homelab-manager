package transport

import (
	"context"
	"math/rand"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const protocolVersion = 1

func Run(ctx context.Context, managerURL, token string, onConnect func(*websocket.Conn) error) error {
	delay := time.Second

	for {
		headers := http.Header{}
		headers.Set("Authorization", "Bearer "+token)
		headers.Set("X-Homelab-Protocol-Version", "1")

		conn, _, err := websocket.Dial(ctx, managerURL, &websocket.DialOptions{HTTPHeader: headers})
		if err == nil {
			delay = time.Second
			err = onConnect(conn)
			_ = conn.Close(websocket.StatusNormalClosure, "context ended")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		jitter := time.Duration(rand.Int63n(int64(delay/2 + 1)))
		timer := time.NewTimer(delay + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
		_ = protocolVersion
	}
}
