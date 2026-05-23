"use client";

import { useEffect, useRef, useState } from "react";
import { useChat } from "@/hooks/use-chat";
import { useAgentContext } from "@/context/agent-context";
import { ActivityRenderer } from "./activity-renderer";
import { PromptInput } from "./prompt-input";
import { StatusIndicator } from "@/components/layout/status-indicator";
import {
  MessageSquare,
  AlertTriangle,
  ChevronDown,
  ChevronUp,
} from "lucide-react";

export function ChatContainer() {
  const { activeAgent, activeSession } = useAgentContext();
  const { activities, isSending, isConnected, error, clearError, send } =
    useChat(activeAgent, activeSession?.key);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [activities, error]);

  if (!activeAgent) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-4">
        <div className="flex size-14 items-center justify-center rounded-2xl bg-muted">
          <MessageSquare className="size-6 text-muted-foreground" />
        </div>
        <div className="text-center">
          <p className="text-base font-medium text-foreground/70">
            Select an agent to start chatting
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Discover or add agents from the sidebar
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Connection status — only show when disconnected */}
      {activeSession && !isConnected && (
        <div className="flex items-center gap-2 border-b border-border/40 px-4 py-1.5">
          <StatusIndicator status="offline" />
          <span className="text-xs text-muted-foreground">Disconnected</span>
        </div>
      )}

      {/* Activity stream */}
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-3xl py-6">
          {!activeSession ? (
            <p className="py-16 text-center text-sm text-zinc-400">
              Preparing a chat session...
            </p>
          ) : activities.length === 0 && !error ? (
            <p className="py-16 text-center text-sm text-zinc-400">
              Send a message to get started.
            </p>
          ) : (
            <>
              <ActivityRenderer activities={activities} />
              {error && <ErrorMessage error={error} onDismiss={clearError} />}
            </>
          )}
          <div ref={bottomRef} />
        </div>
      </div>

      {/* Prompt */}
      <PromptInput
        onSend={send}
        disabled={isSending || !activeSession}
        model={activeAgent?.model}
        provider={activeAgent?.provider}
      />
    </div>
  );
}

function ErrorMessage({
  error,
  onDismiss,
}: {
  error: string;
  onDismiss: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const friendly = humanizeError(error);
  const hasDetails = friendly !== error;

  return (
    <div className="px-5 py-2">
      <div className="rounded-xl border border-red-200 bg-red-50 dark:border-red-900/50 dark:bg-red-950/30">
        <div className="flex items-start gap-3 px-4 py-3">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-red-500" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium text-red-800 dark:text-red-300">
              Something went wrong
            </p>
            <p className="mt-0.5 text-[13px] text-red-600 dark:text-red-400">
              {friendly}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {hasDetails && (
              <button
                onClick={() => setExpanded(!expanded)}
                className="flex items-center gap-0.5 text-[11px] text-red-400 hover:text-red-600 dark:text-red-500 dark:hover:text-red-300"
              >
                {expanded ? "hide" : "details"}
                {expanded ? (
                  <ChevronUp className="size-3" />
                ) : (
                  <ChevronDown className="size-3" />
                )}
              </button>
            )}
            <button
              onClick={onDismiss}
              className="text-[11px] text-red-400 hover:text-red-600 dark:text-red-500 dark:hover:text-red-300"
            >
              dismiss
            </button>
          </div>
        </div>
        {expanded && (
          <div className="border-t border-red-200 px-4 py-2 dark:border-red-900/50">
            <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] text-red-600/80 dark:text-red-400/80">
              {error}
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}

function humanizeError(raw: string): string {
  const providerMatch = raw.match(
    /gateway:.*?http (\d+).*?"message"\s*:\s*"([^"]+)"/,
  );
  if (providerMatch) {
    return `Model provider returned HTTP ${providerMatch[1]}: ${providerMatch[2]}`;
  }
  if (raw.startsWith("ask infer: ")) {
    return humanizeError(raw.slice(11));
  }
  if (raw.length > 200) return raw.slice(0, 200) + "...";
  return raw;
}
