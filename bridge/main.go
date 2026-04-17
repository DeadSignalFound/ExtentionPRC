package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const version = "1.0.0"

var up = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type incoming struct {
	Type     string    `json:"type"`
	ClientID string    `json:"client_id,omitempty"`
	Activity *Activity `json:"activity,omitempty"`
}

type bridge struct {
	mu     sync.Mutex
	ipc    *ipcClient
	cid    string
	ws     *websocket.Conn
	wmu    sync.Mutex
	peer   string
	since  time.Time
	frames uint64
}

func (b *bridge) sendWS(v any) {
	b.wmu.Lock()
	defer b.wmu.Unlock()
	if b.ws == nil {
		return
	}
	_ = b.ws.WriteJSON(v)
}

func (b *bridge) ensure(cid string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ipc != nil && b.cid == cid {
		return nil
	}
	if b.ipc != nil {
		b.ipc.close()
		b.ipc = nil
	}
	c, err := newIPC(cid, func(kind string, payload map[string]any) {
		switch kind {
		case "discord_error":
			msg := "discord error"
			if d, ok := payload["data"].(map[string]any); ok {
				if m, ok := d["message"].(string); ok {
					msg = m
				}
				if code, ok := d["code"].(float64); ok {
					msg = fmt.Sprintf("%s (code %d)", msg, int(code))
				}
			}
			logErr("discord: %s", msg)
			b.sendWS(map[string]string{"status": "error", "msg": "discord: " + msg})
		case "activity_ack":
			b.sendWS(map[string]string{"status": "ok", "msg": "activity set"})
		}
	})
	if err != nil {
		return err
	}
	b.ipc = c
	b.cid = cid
	logDiscord("handshake ok · client_id=%s · pipe=%s", paint(ansiBold, cid), c.pipeName)
	go c.readLoop(func(err error) {
		logWarn("ipc read loop ended: %v", err)
		b.mu.Lock()
		if b.ipc == c {
			b.ipc = nil
		}
		b.mu.Unlock()
		b.sendWS(map[string]string{"status": "error", "msg": "discord ipc closed: " + err.Error()})
	})
	return nil
}

func (b *bridge) set(a *Activity) error {
	b.mu.Lock()
	ic := b.ipc
	b.mu.Unlock()
	if ic == nil {
		return errNoIPC
	}
	transformAssets(a)
	_, err := ic.setActivity(a)
	if err == nil {
		logActivity(a)
	}
	return err
}

func (b *bridge) clear() error {
	b.mu.Lock()
	ic := b.ipc
	b.mu.Unlock()
	if ic == nil {
		return nil
	}
	_, err := ic.setActivity(nil)
	if err == nil {
		logActivity(nil)
	}
	return err
}

var errNoIPC = &ipcErr{"discord ipc not connected"}

type ipcErr struct{ s string }

func (e *ipcErr) Error() string { return e.s }

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := up.Upgrade(w, r, nil)
	if err != nil {
		logErr("ws upgrade: %v", err)
		return
	}
	defer c.Close()

	b := &bridge{ws: c, peer: r.RemoteAddr, since: time.Now()}
	stats.clients.Add(1)
	logWS("%s connected · ua=%q", paint(ansiBold, b.peer), trunc(r.UserAgent(), 48))

	defer func() {
		stats.clients.Add(-1)
		dur := time.Since(b.since).Truncate(time.Second)
		logWS("%s disconnected · %d frames · %s",
			paint(ansiDim, b.peer), b.frames, paint(ansiDim, dur.String()))
		if b.ipc != nil {
			_ = b.clear()
			b.ipc.close()
		}
	}()

	const idleTimeout = 5 * time.Minute
	c.SetReadDeadline(time.Now().Add(idleTimeout))
	c.SetPongHandler(func(string) error {
		c.SetReadDeadline(time.Now().Add(idleTimeout))
		return nil
	})

	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logWarn("ws read: %v", err)
			}
			return
		}
		c.SetReadDeadline(time.Now().Add(idleTimeout))
		b.frames++

		var in incoming
		if err := json.Unmarshal(msg, &in); err != nil {
			writeErr(b, "bad json: "+err.Error())
			continue
		}
		switch in.Type {
		case "connect":
			if in.ClientID == "" {
				writeErr(b, "missing client_id")
				continue
			}
			if err := b.ensure(in.ClientID); err != nil {
				writeErr(b, err.Error())
				continue
			}
			writeOK(b, "connected")
		case "activity":
			if b.ipc == nil {
				writeErr(b, "not connected, send connect first")
				continue
			}
			if err := b.set(in.Activity); err != nil {
				writeErr(b, err.Error())
				continue
			}
			writeOK(b, "activity sent")
		case "clear":
			_ = b.clear()
			writeOK(b, "cleared")
		case "ping":
			writeOK(b, "pong")
		default:
			writeErr(b, "unknown type: "+in.Type)
		}
	}
}

func writeOK(b *bridge, m string) {
	b.sendWS(map[string]string{"status": "ok", "msg": m})
}

func writeErr(b *bridge, m string) {
	logErr("ws → error: %s", m)
	b.sendWS(map[string]string{"status": "error", "msg": m})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:6473", "listen address")
	quiet := flag.Bool("quiet", false, "suppress banner and periodic stats")
	flag.Parse()

	if *quiet {
		useColor = false
	}

	banner(*addr)

	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			logStats()
		}
	}()

	http.HandleFunc("/rpc", wsHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"uptime_s":%d,"clients":%d,"activities":%d,"errors":%d}`,
			int(time.Since(startAt).Seconds()), stats.clients.Load(), stats.activities.Load(), stats.errors.Load())
	})

	logInfo("listening on %s", paint(ansiBold, *addr))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		logErr("listen: %v", err)
	}
}
