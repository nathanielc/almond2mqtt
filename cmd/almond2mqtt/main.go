package main

import (
	"log"
	"os"

	"github.com/nathanielc/almond2mqtt"
)

func main() {

	c := almond2mqtt.Config{
		AlmondAddr:     os.Getenv("ALMOND_ADDR"),
		AlmondUser:     os.Getenv("ALMOND_USER"),
		AlmondPassword: os.Getenv("ALMOND_PASSWORD"),
		MQTTURL:        os.Getenv("MQTT_URL"),
		MQTTPrefix:     os.Getenv("MQTT_PREFIX"),
	}
	c.ApplyDefaults()

	s, err := almond2mqtt.New(c)
	if err != nil {
		log.Fatal(err)
	}
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
