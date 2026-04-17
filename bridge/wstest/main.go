package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

func main() {
	h := http.Header{}
	h.Set("Origin", "moz-extension://11111111-2222-3333-4444-555555555555")
	c, r, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:6473/rpc", h)
	if err != nil {
		log.Fatalf("dial: %v resp=%v", err, r)
	}
	defer c.Close()
	fmt.Println("connected")
	_ = c.WriteJSON(map[string]any{"type": "ping"})
	_, msg, err := c.ReadMessage()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("got:", string(msg))
}
