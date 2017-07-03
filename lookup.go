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

func (l *Lookup) DeviceValue(name, location, value string) (device, index, kind string, found bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, d := range l.dl.Devices {
		if name != "" && name != d.Data["Name"] {
			continue
		}
		if location != "" && location != d.Data["Location"] {
			continue
		}
		if value == "" {
			return d.Data["ID"], "1", d.Data["FriendlyDeviceType"], true
		}
		for i, v := range d.DeviceValues {
			if value != "" && value == v.Name {
				return d.Data["ID"], i, d.Data["FriendlyDeviceType"], true
			}
		}
	}
	return "", "", "", false
}
