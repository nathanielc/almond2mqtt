package marzipan

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

const DefaultHost = "localhost"
const DefaultPort = "7681"

type ClientConfig struct {
	// Host is a host:port adderss of the Almond router.
	// The default is localhost:7681.
	Host     string
	User     string
	Password string
	// Logger is used to log any errors that may occur.
	// If nil no logs will be written.
	Logger *log.Logger
}

// Client provides an API for interacting with the Securifi Almond websocket API.
type Client struct {
	mu        sync.RWMutex
	wg        sync.WaitGroup
	conn      *websocket.Conn
	logger    *log.Logger
	requests  chan request
	responses chan Response
	idx       int
	closing   chan struct{}
	newSubs   chan sub
}

type request struct {
	MobileInternalIndex string
	Request             Request
	ResponseC           chan Response
}

func NewClient(c ClientConfig) (*Client, error) {
	h, p, err := net.SplitHostPort(c.Host)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid host %q", c.Host)
	}
	if h == "" {
		h = DefaultHost
	}
	if p == "" {
		p = DefaultPort
	}
	u := url.URL{
		Scheme: "ws",
		Host:   net.JoinHostPort(h, p),
		Path:   path.Join("/", c.User, c.Password),
	}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(u.String(), http.Header{
		"Origin": []string{"local.host"},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", c.Host)
	}
	logger := c.Logger
	if logger == nil {
		logger = log.New(ioutil.Discard, "", log.LstdFlags)
	}
	cli := &Client{
		conn:      conn,
		closing:   make(chan struct{}),
		requests:  make(chan request),
		responses: make(chan Response),
		newSubs:   make(chan sub),
		logger:    logger,
	}
	cli.wg.Add(1)
	go func() {
		defer cli.wg.Done()
		cli.run()
	}()
	cli.wg.Add(1)
	go func() {
		defer cli.wg.Done()
		cli.readLoop()
	}()
	return cli, nil
}

func (c *Client) Close() {
	c.conn.Close()
	close(c.closing)
	c.wg.Wait()
}

// Request sends a request but does not wait for the response.
func (c *Client) Request(r Request) error {
	_, _, err := c.Do(r)
	return err
}

// Do performs a request and returns a channel were the response can be retrieved.
func (c *Client) Do(r Request) (string, <-chan Response, error) {
	c.mu.Lock()
	c.idx++
	mii := strconv.Itoa(c.idx)
	c.mu.Unlock()

	r.SetMobileInternalIndex(mii)
	req := request{
		MobileInternalIndex: mii,
		Request:             r,
		ResponseC:           make(chan Response, 1),
	}
	select {
	case c.requests <- req:
	case <-c.closing:
		return "", nil, errors.New("client closed")
	}
	return req.MobileInternalIndex, req.ResponseC, nil
}

// Subscribe returns a channel that will receive events for the desired command type.
// If ct is an empty string then responses to all command types are returned.
// Send operations on the channel are non-blocking so events may be dropped.
func (c *Client) Subscribe(ct string) (<-chan Response, error) {
	ch := make(chan Response, 10)
	select {
	case c.newSubs <- sub{ct: ct, ch: ch}:
	case <-c.closing:
		return nil, errors.New("client closed")
	}
	return ch, nil
}

type sub struct {
	ct string
	ch chan Response
}

func (c *Client) readLoop() {
	for {
		r := make(genericResponse)
		if err := c.conn.ReadJSON(&r); err != nil {
			c.logger.Println("failed to read response JSON:", err)
			return
		}
		response, err := r.Response()
		if err != nil {
			c.logger.Println("failed to unmarshal response JSON:", err)
			continue
		}
		select {
		case c.responses <- response:
		case <-c.closing:
			return
		}
	}
}

func (c *Client) run() {
	activeRequests := make(map[string]chan Response)
	subsByCT := make(map[string][]chan Response)
	var globalSubs []chan Response
	for {
		select {
		case <-c.closing:
			return
		case sub := <-c.newSubs:
			if sub.ct == "" {
				globalSubs = append(globalSubs, sub.ch)
			} else {
				subsByCT[sub.ct] = append(subsByCT[sub.ct], sub.ch)
			}
		case r := <-c.requests:
			activeRequests[r.MobileInternalIndex] = r.ResponseC
			if err := c.conn.WriteJSON(r.Request); err != nil {
				c.logger.Printf("failed to write request JSON. MII: %s Error: %v", r.MobileInternalIndex, err)
			}
		case r := <-c.responses:
			mii := r.MobileInternalIndex()
			if rc, ok := activeRequests[mii]; ok {
				rc <- r
				delete(activeRequests, mii)
			}
			ct := r.CommandType()
			subs := subsByCT[ct]
			for _, sub := range subs {
				select {
				case sub <- r:
				default:
				}
			}
			for _, sub := range globalSubs {
				select {
				case sub <- r:
				default:
				}
			}
		}
	}
}

type genericResponse map[string]*json.RawMessage

func (r genericResponse) Meta() Meta {
	return Meta{
		ACT: r.string(Action),
		MII: r.string(MobileInternalIndex),
		CT:  r.string(CommandType),
	}
}

func (r genericResponse) string(k string) string {
	raw, ok := r[k]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(*raw, &s)
	return s
}

func (r genericResponse) unmarshal(k string, v interface{}) error {
	raw, ok := r[k]
	if !ok {
		return fmt.Errorf("unknown key %q", k)
	}
	return json.Unmarshal(*raw, v)
}

func (r genericResponse) Response() (Response, error) {
	commandType := r.string(CommandType)
	switch commandType {
	case "DeviceList":
		dl := &DeviceList{
			Meta:    r.Meta(),
			Devices: make(map[string]Device),
		}
		err := r.unmarshal("Devices", &dl.Devices)
		return dl, err
	case "DynamicIndexUpdated":
		diu := &DynamicIndexUpdated{
			Meta:    r.Meta(),
			Devices: make(map[string]Device),
		}
		err := r.unmarshal("Devices", &diu.Devices)
		return diu, err
	case "DynamicAlmondModeUpdated":
		damu := &DynamicAlmondModeUpdated{
			Meta: r.Meta(),
		}
		if err := r.unmarshal("Mode", &damu.Mode); err != nil {
			return nil, err
		}
		if err := r.unmarshal("EmailId", &damu.EmailId); err != nil {
			return nil, err
		}
		return damu, nil
	case "UpdateDeviceIndex":
		udi := &UpdateDeviceIndex{
			Meta: r.Meta(),
		}
		if err := r.unmarshal("Success", &udi.Success); err != nil {
			return nil, err
		}
		return udi, nil
	case "DynamicClientAdded":
		dca := &DynamicClientAdded{
			Meta: r.Meta(),
		}
		if err := r.unmarshal("Clients", &dca.Clients); err != nil {
			return nil, err
		}
		return dca, nil
	case "DynamicClientJoined":
		dcj := &DynamicClientJoined{
			Meta: r.Meta(),
		}
		if err := r.unmarshal("Clients", &dcj.Clients); err != nil {
			return nil, err
		}
		return dcj, nil
	case "DynamicClientLeft":
		dcl := &DynamicClientLeft{
			Meta: r.Meta(),
		}
		if err := r.unmarshal("Clients", &dcl.Clients); err != nil {
			return nil, err
		}
		return dcl, nil
	default:
		return nil, fmt.Errorf("unsupported command type: %q", commandType)
	}
}
