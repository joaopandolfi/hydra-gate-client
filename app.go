package main

import (
	"bytes"
	"context"
	"crypto/tls"
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
}

func main() {
	config = Config{
		PingFrequency: 10 * time.Second,
		HTTPTimeout:   10 * time.Second,
		PostURL:       "http://localhost:8888/x",
		ServerURL:     "127.0.0.1",
		ServerPort:    8888,
		Secure:        false,
		Token:         "",
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

func post(url string, data []byte) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(data))

	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.Background())
	resp, err := httpC.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)

	return body, err
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
	})
	if err != nil {
		log.Fatal(err)
	}

	_ = c.On("welcome", func(h *gosocketio.Channel, args map[string]interface{}) {
		log.Printf("[+] Welcome message: %v", args)
		log.Println("[-] Authenticating")
		id, _ := uuid.NewUUID()
		c.Emit("register", map[string]string{"id": id.String(), "token": cf.Token})
	})

	_ = c.On("registered", func(h *gosocketio.Channel, args map[string]interface{}) {
		log.Printf("[+] Authenticated -> %v", args["sid"])
	})

	_ = c.On("predict", func(h *gosocketio.Channel, args map[string]interface{}) {
		log.Printf("[+] <== pred: id [%v] timestamp [%v]", args["id"], args["timestamp"])

		payload, _ := json.Marshal(args["data"])

		data, err := post(cf.PostURL, payload)
		if err != nil {
			log.Printf("[-] x=x Error on predict: id [%v] timestamp [%v]", args["id"], time.Now().UTC())
			c.Emit("predicted", map[string]interface{}{"id": args["id"], "success": false, "data": map[string]interface{}{}})
			return
		}

		log.Printf("[+] ==> pred: id [%v] timestamp [%v]", args["id"], time.Now().UTC())
		c.Emit("predicted", map[string]interface{}{"id": args["id"], "success": true, "data": string(data)})
	})

	go ping(c, cf.PingFrequency)
}
