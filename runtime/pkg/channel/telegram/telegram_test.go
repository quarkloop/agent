package telegram

import (
	"context"
	"testing"

	"github.com/quarkloop/runtime/pkg/message"
)

func TestMapUpdateBuildsInternalMessage(t *testing.T) {
	incoming, ok := mapUpdate(update{
		UpdateID: 42,
		Message: &tgMsg{
			Chat: tgChat{ID: 99, Type: "private"},
			Text: "hello",
		},
	})
	if !ok {
		t.Fatal("expected update to map")
	}
	if incoming.SessionID != "telegram-99" || incoming.Title != "private chat" || incoming.Text != "hello" {
		t.Fatalf("unexpected incoming message: %+v", incoming)
	}
}

func TestMapIncomingPostRequestMapsChannelIngress(t *testing.T) {
	got := mapIncomingPostRequest(incomingMessage{SessionID: "telegram-1", Text: "hello"})
	if got.SessionID != "telegram-1" || got.Content != "hello" {
		t.Fatalf("post request = %+v", got)
	}
}

func TestMapUpdateRejectsEmptyMessages(t *testing.T) {
	if _, ok := mapUpdate(update{UpdateID: 1}); ok {
		t.Fatal("nil message should not map")
	}
	if _, ok := mapUpdate(update{UpdateID: 2, Message: &tgMsg{Text: "   "}}); ok {
		t.Fatal("blank message should not map")
	}
}

func TestHandleUpdatePostsThroughAgentFlow(t *testing.T) {
	poster := &fakePoster{closeResponse: true}
	var ensuredID, ensuredType, ensuredTitle string
	ch := New(Config{Token: "unused"}, poster, func(id, channelType, title string) {
		ensuredID = id
		ensuredType = channelType
		ensuredTitle = title
	})

	ch.handleUpdate(context.Background(), update{
		UpdateID: 7,
		Message: &tgMsg{
			Chat: tgChat{ID: 123, Title: "Ops", Type: "group"},
			Text: "status",
		},
	})

	if ensuredID != "telegram-123" || ensuredType != "telegram" || ensuredTitle != "Ops" {
		t.Fatalf("unexpected ensured session: id=%s type=%s title=%s", ensuredID, ensuredType, ensuredTitle)
	}
	if poster.sessionID != "telegram-123" || poster.content != "status" {
		t.Fatalf("unexpected posted message: session=%s content=%s", poster.sessionID, poster.content)
	}
}

func TestHandleUpdateStopsCollectingOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	poster := &fakePoster{}
	ch := New(Config{Token: "unused"}, poster, func(string, string, string) {})

	ch.handleUpdate(ctx, update{
		UpdateID: 8,
		Message: &tgMsg{
			Chat: tgChat{ID: 123, Type: "private"},
			Text: "status",
		},
	})
}

type fakePoster struct {
	closeResponse bool
	sessionID     string
	content       string
}

func (p *fakePoster) Post(ctx context.Context, request message.PostRequest, resp chan message.StreamMessage) {
	p.sessionID = request.SessionID
	p.content = request.Content
	if p.closeResponse {
		close(resp)
	}
}
