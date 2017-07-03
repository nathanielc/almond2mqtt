package marzipan

const (
	Action              = "Action"
	CommandType         = "CommandType"
	MobileInternalIndex = "MobileInternalIndex"
)

type Request interface {
	SetMobileInternalIndex(string)
}

type Response interface {
	MobileInternalIndex() string
	CommandType() string
	Action() string
}

type Meta struct {
	MII string `json:"MobileInternalIndex,omitempty"`
	CT  string `json:"CommandType,omitempty"`
	ACT string `json:"Action,omitempty"`
}

func (m *Meta) MobileInternalIndex() string {
	return m.MII
}
func (m *Meta) SetMobileInternalIndex(mii string) {
	m.MII = mii
}

func (m *Meta) CommandType() string {
	return m.CT
}
func (m *Meta) SetCommandType(ct string) {
	m.CT = ct
}

func (m *Meta) Action() string {
	return m.ACT
}
func (m *Meta) SetAction(a string) {
	m.ACT = a
}

type DeviceList struct {
	Meta
	Devices map[string]Device
}

func NewDeviceListRequest() Request {
	return &Meta{
		CT: "DeviceList",
	}
}

type DynamicIndexUpdated struct {
	Meta
	Devices map[string]Device `json:"Devices"`
}

type Device struct {
	Data         map[string]string      `json:"Data"`
	DeviceValues map[string]DeviceValue `json:"DeviceValues"`
}

type DeviceValue struct {
	Name  string
	Value string
}

type DynamicAlmondModeUpdated struct {
	Meta
	Mode    string
	EmailId string
}

type UpdateDeviceIndexRequest struct {
	Meta
	ID    string `json:"ID"`
	Index string `json:"Index"`
	Value string `json:"Value"`
}

func NewUpdateDeviceIndexRequest(id, index, value string) Request {
	return &UpdateDeviceIndexRequest{
		Meta:  Meta{CT: "UpdateDeviceIndex"},
		ID:    id,
		Index: index,
		Value: value,
	}
}

type UpdateDeviceIndex struct {
	Meta
	Success string `json:"Success"`
}
