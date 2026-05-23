package workflowsvc

import (
	"context"
	"sync"
	"time"

	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
)

type EventLog struct {
	mu       sync.RWMutex
	history  map[string][]*workflowv1.WorkflowEvent
	watchers map[string]map[uint64]chan *workflowv1.WorkflowEvent
	next     uint64
}

func NewEventLog() *EventLog {
	return &EventLog{
		history:  make(map[string][]*workflowv1.WorkflowEvent),
		watchers: make(map[string]map[uint64]chan *workflowv1.WorkflowEvent),
	}
}

func (l *EventLog) Append(event *workflowv1.WorkflowEvent) {
	if l == nil || event == nil || event.GetWorkflowId() == "" {
		return
	}
	copied := cloneEvent(event)
	if copied.OccurredAt == "" {
		copied.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	l.mu.Lock()
	l.history[copied.WorkflowId] = append(l.history[copied.WorkflowId], copied)
	watchers := make([]chan *workflowv1.WorkflowEvent, 0, len(l.watchers[copied.WorkflowId]))
	for _, watcher := range l.watchers[copied.WorkflowId] {
		watchers = append(watchers, watcher)
	}
	l.mu.Unlock()

	for _, watcher := range watchers {
		select {
		case watcher <- cloneEvent(copied):
		default:
		}
	}
}

func (l *EventLog) Stream(ctx context.Context, workflowID string, includeHistory bool) <-chan *workflowv1.WorkflowEvent {
	out := make(chan *workflowv1.WorkflowEvent, 32)
	if l == nil || workflowID == "" {
		close(out)
		return out
	}

	l.mu.Lock()
	if includeHistory {
		for _, event := range l.history[workflowID] {
			out <- cloneEvent(event)
		}
	}
	l.next++
	id := l.next
	watcher := make(chan *workflowv1.WorkflowEvent, 32)
	if l.watchers[workflowID] == nil {
		l.watchers[workflowID] = make(map[uint64]chan *workflowv1.WorkflowEvent)
	}
	l.watchers[workflowID][id] = watcher
	l.mu.Unlock()

	go func() {
		defer close(out)
		defer l.removeWatcher(workflowID, id)
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-watcher:
				select {
				case out <- cloneEvent(event):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func (l *EventLog) removeWatcher(workflowID string, id uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.watchers[workflowID], id)
	if len(l.watchers[workflowID]) == 0 {
		delete(l.watchers, workflowID)
	}
}

func cloneEvent(event *workflowv1.WorkflowEvent) *workflowv1.WorkflowEvent {
	if event == nil {
		return nil
	}
	copied := *event
	return &copied
}
