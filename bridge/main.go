package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var up = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type incoming struct {
	Type     string    `json:"type"`
	ClientID string    `json:"client_id,omitempty"`
	Activity *Activity `json:"activity,omitempty"`
}

type bridge struct {
	mu  sync.Mutex
	ipc *ipcClient
	cid string
	ws  *websocket.Conn
	wmu sync.Mutex
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
	go c.readLoop(func(err error) {
		log.Println("ipc read loop ended:", err)
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
	return err
}

var errNoIPC = &ipcErr{"discord ipc not connected"}

type ipcErr struct{ s string }

func (e *ipcErr) Error() string { return e.s }

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := up.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer c.Close()
	log.Println("client connected:", r.RemoteAddr)

	b := &bridge{ws: c}
	defer func() {
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
			log.Println("read:", err)
			return
		}
		c.SetReadDeadline(time.Now().Add(idleTimeout))
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
	b.sendWS(map[string]string{"status": "error", "msg": m})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:6473", "listen address")
	flag.Parse()

	http.HandleFunc("/rpc", wsHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Println("discord-rpc-bridge listening on", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}
