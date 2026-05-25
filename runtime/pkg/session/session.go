package session

import (
	"sync"
	"time"

	"github.com/quarkloop/runtime/pkg/message"
)

// Conversation represents an in-memory agent communication channel with
// isolated context, message history, and live subscriber notifications.
// This is distinct from the persisted session records and NATS wire contracts
// owned at their respective boundaries.
type Conversation struct {
	mu          sync.RWMutex
	id          string
	sessionType string
	title       string
	status      string
	messages    []message.Message `json:"-"`
	createdAt   string
	updatedAt   string
	subscribers map[chan message.Message]struct{}
}

// ID returns the conversation ID.
func (s *Conversation) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}

// SessionType returns the conversation type (main, chat, subagent, cron).
func (s *Conversation) SessionType() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionType
}

// Title returns the conversation title.
func (s *Conversation) Title() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.title
}

// Status returns the conversation status.
func (s *Conversation) Status() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// CreatedAt returns the creation timestamp.
func (s *Conversation) CreatedAt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.createdAt
}

// UpdatedAt returns the last-update timestamp.
func (s *Conversation) UpdatedAt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

// AddMessage appends a message to the session history and notifies subscribers.
func (s *Conversation) AddMessage(role, content string) {
	msg := message.Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	s.mu.Lock()
	s.messages = append(s.messages, msg)
	subs := make([]chan message.Message, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	// Notify outside lock
	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// GetMessages returns a copy of the session message history.
func (s *Conversation) GetMessages() []message.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]message.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// Subscribe returns a channel that receives new messages added to the session.
func (s *Conversation) Subscribe() chan message.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan message.Message, 16)
	if s.subscribers == nil {
		s.subscribers = make(map[chan message.Message]struct{})
	}
	s.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber channel.
func (s *Conversation) Unsubscribe(ch chan message.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscribers, ch)
}

// Registry manages all sessions for an agent.
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]*Conversation
}

// NewRegistry creates a new session Registry.
func NewRegistry() *Registry {
	return &Registry{sessions: make(map[string]*Conversation)}
}

// GetOrCreate returns an existing session or creates a new one.
func (r *Registry) GetOrCreate(id, sessionType, title string) *Conversation {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[id]; ok {
		return s
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s := &Conversation{
		id:          id,
		sessionType: sessionType,
		title:       title,
		status:      "active",
		createdAt:   now,
		updatedAt:   now,
	}
	r.sessions[id] = s
	return s
}

// Delete removes a session. The session status is set under s.mu to avoid
// a race with concurrent AddMessage holders of s.mu.
func (r *Registry) Delete(id string) {
	r.mu.Lock()
	s, ok := r.sessions[id]
	if ok {
		delete(r.sessions, id)
	}
	r.mu.Unlock()

	if ok {
		s.mu.Lock()
		s.status = "ended"
		s.mu.Unlock()
	}
}

// Get returns a session by ID.
func (r *Registry) Get(id string) *Conversation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[id]
}

// Has returns true if a session with the given ID exists.
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.sessions[id]
	return ok
}

// List returns all active sessions.
func (r *Registry) List() []*Conversation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Conversation, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

// GetMessages returns the message history for a session.
func (r *Registry) GetMessages(id string) []message.Message {
	s := r.Get(id)
	if s == nil {
		return nil
	}
	return s.GetMessages()
}

// Subscribe subscribes to new messages on a session.
func (r *Registry) Subscribe(id string) chan message.Message {
	s := r.Get(id)
	if s == nil {
		return nil
	}
	return s.Subscribe()
}

// Unsubscribe removes a subscriber from a session.
func (r *Registry) Unsubscribe(id string, ch chan message.Message) {
	s := r.Get(id)
	if s == nil {
		return
	}
	s.Unsubscribe(ch)
}
