package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"

	"github.com/Microsoft/go-winio"
)

const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4
)

type Activity struct {
	Name              string      `json:"name,omitempty"`
	Details           string      `json:"details,omitempty"`
	DetailsURL        string      `json:"details_url,omitempty"`
	State             string      `json:"state,omitempty"`
	StateURL          string      `json:"state_url,omitempty"`
	Timestamps        *Timestamps `json:"timestamps,omitempty"`
	Assets            *Assets     `json:"assets,omitempty"`
	Buttons           []Button    `json:"buttons,omitempty"`
	Type              int         `json:"type,omitempty"`
	StatusDisplayType *int        `json:"status_display_type,omitempty"`
}

type Timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	LargeURL   string `json:"large_url,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
	SmallURL   string `json:"small_url,omitempty"`
}

type Button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type ipcClient struct {
	conn     net.Conn
	wmu      sync.Mutex
	clientID string
	pipeName string
	nonce    uint64
	pending  sync.Map
	onEvent  func(string, map[string]any)
}

type reply struct {
	data []byte
	err  error
}

func newIPC(clientID string, onEv func(string, map[string]any)) (*ipcClient, error) {
	c, pipe, err := dialIPC()
	if err != nil {
		return nil, err
	}
	ic := &ipcClient{conn: c, clientID: clientID, pipeName: pipe, onEvent: onEv}
	if err := ic.handshake(); err != nil {
		c.Close()
		return nil, err
	}
	return ic, nil
}

func dialIPC() (net.Conn, string, error) {
	if runtime.GOOS == "windows" {
		for i := 0; i < 10; i++ {
			p := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
			c, err := winio.DialPipe(p, nil)
			if err == nil {
				return c, p, nil
			}
		}
		return nil, "", errors.New("discord ipc pipe not found (is discord running?)")
	}

	roots := []string{os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("TMPDIR"), os.Getenv("TMP"), os.Getenv("TEMP"), "/tmp"}
	for _, r := range roots {
		if r == "" {
			continue
		}
		for i := 0; i < 10; i++ {
			p := fmt.Sprintf("%s/discord-ipc-%d", r, i)
			c, err := net.Dial("unix", p)
			if err == nil {
				return c, p, nil
			}
		}
	}
	return nil, "", errors.New("discord ipc socket not found")
}

func (c *ipcClient) send(op int32, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(op))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

func (c *ipcClient) recv() (int32, []byte, error) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(c.conn, hdr); err != nil {
		return 0, nil, err
	}
	op := int32(binary.LittleEndian.Uint32(hdr[0:4]))
	sz := binary.LittleEndian.Uint32(hdr[4:8])
	buf := make([]byte, sz)
	if _, err := io.ReadFull(c.conn, buf); err != nil {
		return 0, nil, err
	}
	return op, buf, nil
}

func (c *ipcClient) handshake() error {
	p, _ := json.Marshal(map[string]any{
		"v":         1,
		"client_id": c.clientID,
	})
	if err := c.send(opHandshake, p); err != nil {
		return err
	}
	op, data, err := c.recv()
	if err != nil {
		return err
	}
	if op != opFrame {
		return fmt.Errorf("handshake bad op %d: %s", op, string(data))
	}
	var ev map[string]any
	_ = json.Unmarshal(data, &ev)
	if s, _ := ev["evt"].(string); s == "ERROR" {
		return fmt.Errorf("discord rejected handshake: %s", string(data))
	}
	return nil
}

func (c *ipcClient) setActivity(a *Activity) (string, error) {
	c.nonce++
	nonce := fmt.Sprintf("n-%d", c.nonce)
	cmd := map[string]any{
		"cmd":   "SET_ACTIVITY",
		"nonce": nonce,
		"args": map[string]any{
			"pid":      os.Getpid(),
			"activity": a,
		},
	}
	if a == nil {
		cmd["args"] = map[string]any{"pid": os.Getpid()}
	}
	p, err := json.Marshal(cmd)
	if err != nil {
		return "", err
	}

	ch := make(chan reply, 1)
	c.pending.Store(nonce, ch)
	defer c.pending.Delete(nonce)

	if err := c.send(opFrame, p); err != nil {
		return "", err
	}
	return nonce, nil
}

func (c *ipcClient) readLoop(onErr func(error)) {
	for {
		op, data, err := c.recv()
		if err != nil {
			onErr(err)
			return
		}
		switch op {
		case opPing:
			_ = c.send(opPong, data)
		case opFrame:
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				logWarn("ipc bad frame: %v", err)
				continue
			}
			evt, _ := m["evt"].(string)
			cmd, _ := m["cmd"].(string)
			nonce, _ := m["nonce"].(string)
			if evt == "ERROR" {
				logErr("ipc error (cmd=%s nonce=%s): %s", cmd, nonce, string(data))
				if c.onEvent != nil {
					c.onEvent("discord_error", m)
				}
			} else if cmd == "SET_ACTIVITY" {
				if c.onEvent != nil {
					c.onEvent("activity_ack", m)
				}
			} else if cmd == "DISPATCH" {
				logDiscord("dispatch evt=%s", evt)
			}
		case opClose:
			logWarn("ipc close frame: %s", string(data))
			onErr(fmt.Errorf("ipc closed by discord: %s", string(data)))
			return
		}
	}
}

func (c *ipcClient) close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
