package almond2mqtt

type Config struct {
	AlmondAddr     string
	AlmondUser     string
	AlmondPassword string
	MQTTURL        string
	MQTTPrefix     string
}

func (c *Config) ApplyDefaults() {
	if c.MQTTURL == "" {
		c.MQTTURL = "tcp://localhost:1883"
	}
	if c.MQTTPrefix == "" {
		c.MQTTPrefix = "almond"
	}
}
