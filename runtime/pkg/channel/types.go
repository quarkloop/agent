package channel

// ChannelInfo describes a channel for the API response.
type ChannelInfo struct {
	Type   ChannelType `json:"type"`
	Active bool        `json:"active"`
}

// AllChannelTypes is the list of all known channel types.
var AllChannelTypes = []ChannelType{
	WebChannelType,
	TelegramChannelType,
	NATSChannelType,
}
