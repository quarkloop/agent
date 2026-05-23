"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAgentContext } from "@/context/agent-context";
import {
  useSessions,
  useCreateSession,
  useDeleteSession,
} from "@/hooks/use-sessions-query";
import { Button } from "@/components/themed/button";
import { Plus, Trash2, MessageCircle, Bot } from "lucide-react";

export function SessionSidebar() {
  const { dispatch, activeAgent, activeSession } = useAgentContext();
  const initializedAgentRef = useRef<string | null>(null);

  const { data: sessions = [], isSuccess } = useSessions(
    activeAgent?.id,
    activeAgent?.spaceId,
  );
  const { mutate: createSession, isPending: isCreatingSession } =
    useCreateSession(activeAgent?.id ?? "", activeAgent?.spaceId);
  const { mutate: deleteSession } = useDeleteSession(
    activeAgent?.id ?? "",
    activeAgent?.spaceId,
  );
  const syncedSessionRef = useRef<string | null>(null);
  const sessionSignature = sessions
    .map((session) => `${session.key}:${session.updated_at}`)
    .join("|");

  useEffect(() => {
    if (!activeAgent || !isSuccess) return;
    const syncKey = `${activeAgent.id}:${sessionSignature}`;
    if (syncedSessionRef.current !== syncKey) {
      syncedSessionRef.current = syncKey;
      dispatch({ type: "SET_SESSIONS", sessions });
    }

    if (
      sessions.length === 0 &&
      initializedAgentRef.current !== activeAgent.id &&
      !isCreatingSession
    ) {
      initializedAgentRef.current = activeAgent.id;
      createSession({ type: "chat", title: "Chat" });
    }
  }, [
    activeAgent,
    createSession,
    dispatch,
    isCreatingSession,
    isSuccess,
    sessionSignature,
    sessions,
  ]);

  const handleCreate = useCallback(() => {
    createSession({ type: "chat" });
  }, [createSession]);

  const handleDelete = useCallback(
    (key: string) => {
      deleteSession(key);
    },
    [deleteSession],
  );

  if (!activeAgent) return null;

  const visible = sessions.filter(
    (s) => s.type === "main" || s.type === "chat",
  );

  return (
    <div className="flex flex-col border-t border-border/60 px-2 py-2">
      <div className="flex items-center justify-between px-2 pb-1">
        <span className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
          Sessions
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={handleCreate}
          aria-label="New chat session"
          className="size-6 text-muted-foreground hover:text-foreground"
        >
          <Plus className="size-3" />
        </Button>
      </div>
      <div className="space-y-0.5">
        {visible.map((session) => (
          <div
            key={session.key}
            className={`group flex items-center gap-2 rounded-md px-2 py-1.5 text-sm cursor-pointer transition-colors ${
              activeSession?.key === session.key
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
            onClick={() =>
              dispatch({ type: "SET_ACTIVE_SESSION", key: session.key })
            }
          >
            {session.type === "main" ? (
              <Bot className="size-3.5 shrink-0" />
            ) : (
              <MessageCircle className="size-3.5 shrink-0" />
            )}
            <span className="flex-1 truncate text-xs">
              {session.title || (session.type === "main" ? "Main" : "Chat")}
            </span>
            {session.type !== "main" && (
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={(e) => {
                  e.stopPropagation();
                  handleDelete(session.key);
                }}
                className="size-5 opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-destructive"
                aria-label="Delete session"
              >
                <Trash2 className="size-3" />
              </Button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
