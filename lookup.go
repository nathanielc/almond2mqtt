package almond2mqtt

import (
	"sync"

	"github.com/nathanielc/marzipan"
)

type Lookup struct {
	mu sync.RWMutex
	dl *marzipan.DeviceList
}

func (l *Lookup) Update(dl *marzipan.DeviceList) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dl = dl
}

// DeviceValue returns the device, index, kind and whether or not the device was found.
func (l *Lookup) DeviceValue(name, dtype, location string) (string, string, string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, d := range l.dl.Devices {
		if name != "" && name != d.Data["Name"] {
			continue
		}
		if location != "" && location != d.Data["Location"] {
			continue
		}
		if dtype != "" && dtype != d.Data["FriendlyDeviceType"] {
			continue
		}
		return d.Data["ID"], "1", d.Data["FriendlyDeviceType"], true
	}
	return "", "", "", false
}

// DeviceValue returns the device, index, kind and whether or not the device was found.
func (l *Lookup) Device(key string) (marzipan.Device, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	d, ok := l.dl.Devices[key]
	return d, ok
}
