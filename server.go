package almond2mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/nathanielc/marzipan"
)

const (
	statusTopic = "status"
	setTopic    = "set"
)

type Server struct {
	prefix           string
	setTopic         string
	setTopicAnchored string
	mqttC            mqtt.Client
	almondC          *marzipan.Client
	lookup           *Lookup

	logger *log.Logger
}

func New(c Config) (*Server, error) {
	// Connect to MQTT
	mqtt.ERROR = log.New(os.Stderr, "[mqtt] E ", 0)
	opts := mqtt.NewClientOptions().AddBroker(c.MQTTURL).SetClientID("almond2mqtt")
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	mqttC := mqtt.NewClient(opts)
	if token := mqttC.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	// Connect to Almond
	logger := log.New(os.Stderr, "[almond] ", log.LstdFlags)
	almondC, err := marzipan.NewClient(marzipan.ClientConfig{
		Host:     c.AlmondAddr,
		User:     c.AlmondUser,
		Password: c.AlmondPassword,
		Logger:   logger,
	})
	if err != nil {
		return nil, err
	}

	setTopic := path.Join(c.MQTTPrefix, setTopic)
	return &Server{
		prefix:           c.MQTTPrefix,
		setTopic:         setTopic,
		setTopicAnchored: setTopic + "/",
		mqttC:            mqttC,
		almondC:          almondC,
		lookup:           new(Lookup),
		logger:           log.New(os.Stderr, "[server] ", log.LstdFlags),
	}, nil
}

func (s *Server) Run() error {
	s.logger.Println("running...")
	// Subscribe to all set messages
	if token := s.mqttC.Subscribe(path.Join(s.setTopic, "#"), 0, s.handleSetMessage); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	// Subscribe to all Almond events
	sub, err := s.almondC.Subscribe("")
	if err != nil {
		return err
	}

	// Always send device list on startup
	s.almondC.Request(marzipan.NewDeviceListRequest())
	// Periodically list all devices
	go func() {
		ticker := time.NewTicker(time.Hour)
		for range ticker.C {
			s.almondC.Request(marzipan.NewDeviceListRequest())
		}
	}()

	// Forward all events to MQTT bus
	for r := range sub {
		payload, err := json.Marshal(r)
		if err != nil {
			return err
		}
		topic := path.Join(s.prefix, statusTopic, r.CommandType())
		if r.CommandType() == "DeviceList" {
			dl, ok := r.(*marzipan.DeviceList)
			if ok {
				s.lookup.Update(dl)
			}
		}

		if token := s.mqttC.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
			return token.Error()
		}
	}
	return nil
}

func (s *Server) handleSetMessage(c mqtt.Client, m mqtt.Message) {
	p := strings.TrimPrefix(m.Topic(), s.setTopicAnchored)
	var typ, resource string
	if i := strings.Index(p, "/"); i > 0 {
		typ = p[:i]
		resource = p[i+1:]
	} else {
		typ = p
	}
	switch typ {
	case "device":
		if err := s.updateDevice(resource, string(m.Payload())); err != nil {
			s.logger.Println("failed to update device", err)
		}
	default:
		s.logger.Println("unknown set typ", typ)
	}
}

func (s *Server) updateDevice(resource, data string) error {
	var name, location, valueName string
	parts := strings.Split(resource, "/")
	switch l := len(parts); l {
	case 1:
		name = parts[0]
	case 2:
		name = parts[0]
		location = parts[1]
	case 3:
		location = parts[0]
		name = parts[1]
		valueName = parts[2]
	default:
		return fmt.Errorf("resource must have 1, 2 or 3 parts %q", resource)
	}
	id, index, kind, ok := s.lookup.DeviceValue(name, location, valueName)
	if !ok {
		return fmt.Errorf("could not find device %q", resource)
	}
	value := s.determineValueForKind(kind, data)

	r := marzipan.NewUpdateDeviceIndexRequest(id, index, value)
	return s.almondC.Request(r)
}

func (s *Server) determineValueForKind(kind, data string) string {
	switch kind {
	case "MultilevelSwitch":
		switch data {
		case "on":
			return "100"
		case "off":
			return "0"
		}
	case "DoorLock":
		switch data {
		case "lock", "close":
			return "255"
		case "unlock", "open":
			return "0"
		}
	}
	return data
}
