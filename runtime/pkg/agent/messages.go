// Package agent provides typed messages for the agent loop.
package agent

import (
	"context"

	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/message"

	"github.com/quarkloop/pkg/plugin"
)

// Message type constants.
const (
	MsgTypeUserMessage = "user_message"
	MsgTypeInitLLM     = "init_llm"
	MsgTypeInitChannel = "init_channel"
	MsgTypeWorkStep    = "work_step"
	MsgTypeSetModel    = "set_model"
	MsgTypeToolCall    = "tool_call"
)

// UserMessageMsg carries an incoming user message.
type UserMessageMsg struct {
	loop.BaseMessage
	Context   context.Context
	SessionID string
	Content   string
	Response  chan message.StreamMessage
}

// NewUserMessage creates a new user message.
func NewUserMessage(ctx context.Context, request message.PostRequest, resp chan message.StreamMessage) UserMessageMsg {
	if ctx == nil {
		ctx = context.Background()
	}
	return UserMessageMsg{
		BaseMessage: loop.NewMessage(MsgTypeUserMessage),
		Context:     ctx,
		SessionID:   request.SessionID,
		Content:     request.Content,
		Response:    resp,
	}
}

// InitLLMMsg carries LLM initialization data.
type InitLLMMsg struct {
	loop.BaseMessage
	ModelListURL string
	Providers    map[string]plugin.Provider // typed providers
	Fallback     []plugin.ModelEntry        // typed model entries
}

// NewInitLLMMsg creates a new InitLLM message.
func NewInitLLMMsg() InitLLMMsg {
	return InitLLMMsg{
		BaseMessage: loop.NewMessage(MsgTypeInitLLM),
	}
}

// InitChannelMsg carries channel initialization data.
type InitChannelMsg struct {
	loop.BaseMessage
	Bus any // *channel.ChannelBus
}

// NewInitChannelMsg creates a new InitChannel message.
func NewInitChannelMsg(bus any) InitChannelMsg {
	return InitChannelMsg{
		BaseMessage: loop.NewMessage(MsgTypeInitChannel),
		Bus:         bus,
	}
}

// WorkStepMsg triggers work step execution.
type WorkStepMsg struct {
	loop.BaseMessage
}

// NewWorkStepMsg creates a new WorkStep message with high priority.
func NewWorkStepMsg() WorkStepMsg {
	return WorkStepMsg{
		BaseMessage: loop.NewPriorityMessage(MsgTypeWorkStep, 10), // High priority
	}
}

// SetModelMsg carries the model ID to switch to.
type SetModelMsg struct {
	loop.BaseMessage
	ModelID string
}

// NewSetModelMsg creates a new SetModel message.
func NewSetModelMsg(modelID string) SetModelMsg {
	return SetModelMsg{
		BaseMessage: loop.NewMessage(MsgTypeSetModel),
		ModelID:     modelID,
	}
}

// ToolCallMsg represents a tool execution request.
// Implements execution.ToolCallMessage for middleware interception.
type ToolCallMsg struct {
	loop.BaseMessage
	Tool       string
	Arguments  string
	Session    string
	ResultChan chan AgentToolResult
}

// AgentToolResult holds the result of a tool execution.
type AgentToolResult struct {
	Output string
	Error  error
}

// NewToolCallMsg creates a new ToolCall message.
func NewToolCallMsg(tool, arguments, sessionID string) ToolCallMsg {
	return ToolCallMsg{
		BaseMessage: loop.NewPriorityMessage(MsgTypeToolCall, 5),
		Tool:        tool,
		Arguments:   arguments,
		Session:     sessionID,
		ResultChan:  make(chan AgentToolResult, 1),
	}
}

// ToolName returns the tool name for permission checking.
func (m ToolCallMsg) ToolName() string { return m.Tool }

// ToolArguments returns the tool arguments for approval display.
func (m ToolCallMsg) ToolArguments() string { return m.Arguments }

// SessionID returns the session ID for approval tracking.
func (m ToolCallMsg) SessionID() string { return m.Session }
