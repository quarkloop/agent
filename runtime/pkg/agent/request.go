package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/quarkloop/runtime/pkg/guard"
	"github.com/quarkloop/runtime/pkg/llm"
	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"github.com/quarkloop/runtime/pkg/workflow"
)

// handleUserMessage owns one user-visible inference request and its session
// history transition. Tool and activity concerns are delegated to collaborators.
func (a *Agent) handleUserMessage(ctx context.Context, msg loop.Message) error {
	userMsg := msg.(UserMessageMsg)
	defer close(userMsg.Response)

	runID := newRunID()
	requestCtx, cancel := context.WithCancel(ctx)
	spaceID := userMsg.SpaceID
	if spaceID == "" {
		spaceID = a.Space
	}
	requestCtx = modelservice.WithSpaceID(requestCtx, spaceID)
	requestCtx = modelservice.WithSessionID(requestCtx, userMsg.SessionID)
	requestCtx = modelservice.WithRunID(requestCtx, runID)
	defer cancel()
	stopRequestCancel := context.AfterFunc(userMsg.Context, cancel)
	defer stopRequestCancel()

	response := userMsg.Response
	if a.Activity != nil {
		a.addActivity(userMsg.SessionID, "message.user", map[string]any{
			"content_length": len(userMsg.Content),
			"run_id":         runID,
		})
		instrumented, stopForwarding := a.instrumentResponse(requestCtx, userMsg.SessionID, userMsg.Response)
		response = instrumented
		defer stopForwarding()
	}
	slog.Info("agent message started", "session_id", userMsg.SessionID, "run_id", runID, "content_length", len(userMsg.Content))

	s := a.Sessions.Get(userMsg.SessionID)
	if s == nil {
		s = a.Sessions.GetOrCreate(userMsg.SessionID, "chat", "")
	}
	s.AddMessage("user", userMsg.Content)

	client := a.Models.GetDefault()
	if client == nil {
		return fmt.Errorf("no LLM client configured")
	}
	if a.config.ToolCallArguments != nil {
		requestCtx = llm.WithToolCallArgumentNormalizer(requestCtx, a.config.ToolCallArguments)
	}

	history := s.GetMessages()
	tools := a.defaultTools()
	workflowTracker := workflow.NewTracker(userMsg.SessionID, userMsg.Content, tools, a.Workflows, func(event workflow.Event) {
		a.addActivity(userMsg.SessionID, "workflow."+event.Type, event)
	})
	systemPrompt := a.systemPrompt()
	onTool := a.executeTool
	var workflowGuard llm.FinalGuard
	var workflowToolCallGate llm.ToolCallGate
	var workflowToolCallGuard llm.ToolCallGuard
	var workflowToolResultGate llm.ToolResultGate
	var workflowToolResultInstruction llm.ToolResultInstruction
	if workflowTracker != nil {
		if block := workflowTracker.PromptBlock(); block != "" {
			systemPrompt += "\n\n" + block
		}
		onTool = workflowTracker.WrapToolHandler(onTool)
		workflowGuard = workflowTracker.FinalGuard
		workflowToolCallGate = workflowTracker.AcceptFinalBeforeToolCalls
		workflowToolCallGuard = workflowTracker.GuardToolCalls
		workflowToolResultGate = workflowTracker.AcceptFinalAfterToolCalls
		workflowToolResultInstruction = workflowTracker.ContinuationPrompt
	}
	fullResponse, err := message.HandleWithToolCallPolicyAndContinuation(
		requestCtx,
		history,
		client,
		systemPrompt,
		a.Plan.GetSummary(),
		tools,
		onTool,
		response,
		guard.CombineFinalGuards(a.finalGuard(), workflowGuard),
		workflowToolCallGate,
		workflowToolCallGuard,
		workflowToolResultGate,
		workflowToolResultInstruction,
	)
	if err != nil {
		a.emitMessageError(requestCtx, userMsg.SessionID, response, err)
		slog.Error("agent message failed", "session_id", userMsg.SessionID, "run_id", runID, "error", err)
		return err
	}
	if a.config.PendingRefs != nil {
		if refs := a.config.PendingRefs(); len(refs) > 0 {
			err := guard.UnconsumedPendingRefsError(refs)
			a.emitMessageError(requestCtx, userMsg.SessionID, response, err)
			slog.Error("agent message failed", "session_id", userMsg.SessionID, "run_id", runID, "error", err)
			return err
		}
	}

	s.AddMessage("assistant", fullResponse)
	if a.Activity != nil {
		a.addActivity(userMsg.SessionID, "message.assistant", map[string]any{
			"content_length": len(fullResponse),
			"run_id":         runID,
		})
	}
	slog.Info("agent message completed", "session_id", userMsg.SessionID, "run_id", runID, "content_length", len(fullResponse))
	return nil
}

func newRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
}
