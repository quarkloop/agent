package channel

import (
	"context"
	"testing"
)

type testChannel struct {
	channelType ChannelType
	started     bool
	stopped     bool
}

func (c *testChannel) Type() ChannelType { return c.channelType }

func (c *testChannel) Start(context.Context) error {
	c.started = true
	return nil
}

func (c *testChannel) Stop(context.Context) error {
	c.stopped = true
	return nil
}

func TestChannelBusSupportsMultipleChannelsWithSameType(t *testing.T) {
	bus := NewChannelBus()
	first := &testChannel{channelType: NATSChannelType}
	second := &testChannel{channelType: NATSChannelType}

	bus.Register(first)
	bus.Register(second)

	if got := bus.Get(NATSChannelType); got != first {
		t.Fatalf("first channel = %#v, want %#v", got, first)
	}
	active := bus.ActiveChannels()
	if len(active) != 2 {
		t.Fatalf("active channels = %+v", active)
	}
	if err := bus.Start(context.Background()); err != nil {
		t.Fatalf("start bus: %v", err)
	}
	if !first.started || !second.started {
		t.Fatalf("started first=%t second=%t", first.started, second.started)
	}
	if err := bus.Stop(context.Background()); err != nil {
		t.Fatalf("stop bus: %v", err)
	}
	if !first.stopped || !second.stopped {
		t.Fatalf("stopped first=%t second=%t", first.stopped, second.stopped)
	}
}
