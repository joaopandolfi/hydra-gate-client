package main

import (
	"bytes"
	"context"
	"crypto/tls"
	b64 "encoding/base64"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"
	gosocketio "github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
	"github.com/segmentio/encoding/json"
)

var cx chan int
var httpC *http.Client
var config Config

type Config struct {
	PostURL       string
	ServerURL     string
	ServerPort    int
	Secure        bool
	HTTPTimeout   time.Duration
	PingFrequency time.Duration
	Token         string
	ID            string
}

type response struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Data    string      `json:"data"`
	Header  http.Header `json:"header"`
}

func main() {
	uid, _ := uuid.NewUUID()
	config = Config{
		PingFrequency: 10 * time.Second,
		HTTPTimeout:   10 * time.Second,
		PostURL:       "https://google.com",
		ServerURL:     "127.0.0.1:1223/socket.io/?EIO=3&transport=websocket",
		ServerPort:    1223,
		Secure:        false,
		Token:         "x",
		ID:            uid.String(),
	}

	cx := make(chan int, 1)
	runtime.GOMAXPROCS(runtime.NumCPU())

	httpClient(config.HTTPTimeout)

	go sockRoutine(&config)

	log.Printf("[x] Exiting signal: %v", <-cx)
}

func httpClient(timeout time.Duration) {
	if httpC == nil {
		httpC = &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}
}

func sendHTTP(url, path, method string, data []byte) ([]byte, http.Header, error) {
	req, _ := http.NewRequest(method, url+path, bytes.NewBuffer(data))

	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.Background())
	resp, err := httpC.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)

	return body, resp.Header, err
}

func ping(c *gosocketio.Client, frequency time.Duration) {
	for c.IsAlive() {
		time.Sleep(frequency)
		c.Emit("ping", "")
	}
}

func sockRoutine(cf *Config) {
	log.Println("[+] Connecting")
	c, err := gosocketio.Dial(
		gosocketio.GetUrl(cf.ServerURL, cf.ServerPort, cf.Secure),
		transport.GetDefaultWebsocketTransport())
	if err != nil {
		log.Printf("[x] Error: %v", err)
		time.Sleep(cf.PingFrequency)
		go sockRoutine(cf)
		return
	}

	c.On(gosocketio.OnError, func(h *gosocketio.Channel, args interface{}) {
		log.Printf("[x] Error %v", args)
		cx <- 1
	})
	err = c.On(gosocketio.OnDisconnection, func(h *gosocketio.Channel, args interface{}) {
		log.Printf("[-] Disconnected %v", args)
		time.Sleep(cf.PingFrequency)
		go sockRoutine(cf)
	})
	if err != nil {
		log.Printf("Got error: %v", err)
		cx <- 1
	}

	err = c.On(gosocketio.OnConnection, func(h *gosocketio.Channel) {
		log.Println("[+] Connected")
		log.Println("[-] Authenticating")
		c.Emit("register", map[string]string{"id": cf.ID, "token": cf.Token})
	})
	if err != nil {
		log.Fatal(err)
	}

	_ = c.On("welcomes", func(h *gosocketio.Channel, args map[string]interface{}) {
		log.Printf("[+] Welcome message: %v", args)
		log.Println("[-] Authenticating")
		id, _ := uuid.NewUUID()
		c.Emit("register", map[string]string{"id": id.String(), "token": cf.Token})
	})

	_ = c.On("registered", func(h *gosocketio.Channel, args map[string]interface{}) {
		log.Printf("[+] Authenticated -> %v -> %v", args["sid"], cf.ID)
	})

	_ = c.On("handle", func(h *gosocketio.Channel, args map[string]interface{}) {
		log.Printf("[+] <== handle: id [%v] timestamp [%v] method [%v] path[%v]", args["id"], args["timestamp"], args["method"], args["path"])

		payload, _ := json.Marshal(args["data"])

		data, head, err := sendHTTP(cf.PostURL, args["path"].(string), args["method"].(string), payload)
		if err != nil {
			log.Printf("[-] x=x Error on handle: id [%v] timestamp [%v] method[%v] path[%v]", args["id"], time.Now().UTC(), args["method"], args["path"])
			c.Emit("response", response{ID: args["id"].(string), Success: false, Data: ""})
			return
		}

		log.Printf("[+] ==> pred: id [%v] timestamp [%v] path [%v]", args["id"], time.Now().UTC(), args["path"].(string))
		datab64 := b64.StdEncoding.EncodeToString(data)
		c.Emit("response", response{ID: args["id"].(string), Success: true, Data: datab64, Header: head})
	})

	go ping(c, cf.PingFrequency)
}
