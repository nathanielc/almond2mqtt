package almond2mqtt

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/nathanielc/marzipan"
	"github.com/nathanielc/smarthome"
)

type Server struct {
	prefix  string
	home    smarthome.Server
	almondC *marzipan.Client
	lookup  *Lookup

	logger *log.Logger
}

func New(c Config) (*Server, error) {
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

	s := &Server{
		prefix:  c.MQTTPrefix,
		almondC: almondC,
		lookup:  new(Lookup),
		logger:  log.New(os.Stderr, "[server] ", log.LstdFlags),
	}

	// Connect to MQTT
	mqtt.ERROR = log.New(os.Stderr, "[mqtt] E ", 0)
	opts := smarthome.DefaultMQTTClientOptions().
		AddBroker(c.MQTTURL).
		SetClientID("almond2mqtt")
	s.home = smarthome.NewServer(s.prefix, s, opts)
	return s, nil
}

func (s *Server) Run() error {

	if err := s.home.Connect(); err != nil {
		return err
	}

	s.home.PublishHWStatus(smarthome.Disconnected)

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

	s.home.PublishHWStatus(smarthome.Connected)

	s.logger.Println("running...")

	// Forward all events to MQTT bus
	for r := range sub {
		if r.CommandType() == "DeviceList" {
			dl, ok := r.(*marzipan.DeviceList)
			if ok {
				s.lookup.Update(dl)
			}
		}
		s.publishStatus(r)
	}
	return nil
}

func (s *Server) publishStatus(r marzipan.Response) {
	switch m := r.(type) {
	case *marzipan.DeviceList:
		s.publishDevicesStatus(m.Devices)
	case *marzipan.DynamicIndexUpdated:
		s.publishDevicesStatus(m.Devices)
	}
}

func (s *Server) publishDevicesStatus(devices map[string]marzipan.Device) {
	for key, device := range devices {
		data := device.Data
		if len(data) == 0 {
			// Device has no local data, maybe we have it cached
			c, ok := s.lookup.Device(key)
			if ok {
				data = c.Data
			} else {
				// We can't create a valid device path without the data.
				continue
			}
		}
		dp := devicePathFromDeviceData(data)
		dv := device.DeviceValues["1"]
		if dv.Value != "" {
			v := smarthome.Value{
				Value: s.mqttValueForType(data["FriendlyDeviceType"], dv.Value),
			}
			s.home.PublishStatus(dp, v)
		}
	}
}

func devicePathFromDeviceData(data map[string]string) string {
	return path.Join(data["Name"], data["FriendlyDeviceType"], data["Location"])
}
func devicePathToDeviceData(item string) (name, dtype, location string, err error) {
	parts := strings.Split(item, "/")
	if len(parts) != 3 {
		err = fmt.Errorf("invalid device path %q", item)
		return
	}
	name = parts[0]
	dtype = parts[1]
	location = parts[2]
	return
}

func (s *Server) updateDevice(resource, data string) error {
	name, dtype, location, err := devicePathToDeviceData(resource)
	if err != nil {
		return err
	}
	id, index, dtype, ok := s.lookup.DeviceValue(name, dtype, location)
	if !ok {
		return fmt.Errorf("could not find device %q", resource)
	}
	value := s.almondValueForType(dtype, data)

	r := marzipan.NewUpdateDeviceIndexRequest(id, index, value)
	return s.almondC.Request(r)
}

func (s *Server) Set(toplevel, item string, value interface{}) {
	vStr := fmt.Sprintf("%v", value)
	if err := s.updateDevice(item, vStr); err != nil {
		s.logger.Println("failed to update device", err)
	}
}
func (s *Server) Get(toplevel, item string) (smarthome.Value, bool) {
	return smarthome.Value{}, false
}
func (s *Server) Command(toplevel string, cmd []byte) {}

func (s *Server) almondValueForType(dtype, data string) string {
	switch dtype {
	case "BinaryPowerSwitch", "BinarySwitch", "SmartACSwitch":
		switch data {
		case "on":
			return "true"
		case "off":
			return "false"
		}
	case "MultilevelSwitch":
		switch data {
		case "on":
			return "100"
		case "off":
			return "0"
		}
	case "DoorLock":
		switch data {
		case "on", "locked":
			return "255"
		case "off", "unlocked":
			return "0"
		}
	}
	return data
}

func (s *Server) mqttValueForType(dtype, data string) string {
	switch dtype {
	case "BinaryPowerSwitch", "BinarySwitch", "SmartACSwitch":
		switch data {
		case "true":
			return "on"
		case "false":
			return "off"
		}
	case "MultilevelSwitch":
		switch data {
		case "100":
			return "on"
		case "0":
			return "off"
		}
	case "DoorLock":
		switch data {
		case "255":
			return "locked"
		case "0":
			return "unlocked"
		}
	}
	return data
}
