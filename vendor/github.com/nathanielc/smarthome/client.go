package smarthome

import (
	"path"

	"github.com/eclipse/paho.mqtt.golang"
)

type SubscribeCallback func(toplevel, item string, value Value)

type Client interface {
	// Set publishes a set message with the value
	Set(toplevel, item string, value string) error
	// Get publishes a get request message.
	// You must Subscribe before calls to Get in order to receive the response.
	Get(toplevel, item string) error
	// Command publishes a command to the toplevel topic.
	Command(toplevel string, cmd []byte) error

	// Subscribe to receive callbacks whenever a status message is received.
	Subscribe(toplevel, item string, callback SubscribeCallback) error
	// Unsubscribe to stop receiving subscription callbacks.
	Unsubscribe(toplevel, item string) error
}

type client struct {
	c mqtt.Client
}

func NewClient(opts *mqtt.ClientOptions) (Client, error) {
	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}
	return &client{
		c: c,
	}, nil
}

func (c *client) Set(toplevel, item string, value string) error {
	topic := path.Join(toplevel, setPath, item)
	token := c.c.Publish(topic, 0, false, value)
	token.Wait()
	return token.Error()
}

func (c *client) Get(toplevel, item string) error {
	getTopic := path.Join(toplevel, getPath, item)
	token := c.c.Publish(getTopic, 0, false, "?")
	token.Wait()
	return token.Error()
}

func (c *client) Command(toplevel string, cmd []byte) error {
	topic := path.Join(toplevel, commandPath)
	token := c.c.Publish(topic, 0, false, cmd)
	token.Wait()
	return token.Error()
}

func (c *client) Subscribe(toplevel, item string, callback SubscribeCallback) error {
	topic := path.Join(toplevel, statusPath, item)
	token := c.c.Subscribe(topic, 0, func(c mqtt.Client, m mqtt.Message) {
		value := PayloadToValue(m.Payload())
		callback(toplevel, item, value)
	})
	token.Wait()
	return token.Error()
}
func (c *client) Unsubscribe(toplevel, item string) error {
	topic := path.Join(toplevel, statusPath, item)
	token := c.c.Unsubscribe(topic)
	token.Wait()
	return token.Error()
}
