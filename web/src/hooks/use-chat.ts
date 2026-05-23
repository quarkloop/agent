"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type {
  ActivityRecord,
  AgentConnection,
  AgentMode,
  FileAttachment,
} from "@/lib/types";
import { activityKey, useActivity } from "@/hooks/use-activity-query";
import { useSendMessage } from "@/hooks/use-chat-query";
import {
  assistantMessageActivity,
  eventText,
  sessionEventToActivity,
  userMessageActivity,
} from "@/lib/nats/activity";
import { subscribeSessionEvents } from "@/lib/nats/client";

export function useChat(
  agent: AgentConnection | undefined,
  sessionKey?: string | null,
) {
  const [liveActivities, setLiveActivities] = useState<ActivityRecord[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [streamError, setStreamError] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const sendMut = useSendMessage(agent?.id, agent?.spaceId);
  const { data: history = [] } = useActivity(
    agent?.id,
    sessionKey,
    agent?.spaceId,
  );

  const allActivities = useMemo(() => {
    const byID = new Map<string, ActivityRecord>();
    for (const activity of [...history, ...liveActivities]) {
      byID.set(activity.id, activity);
    }
    return Array.from(byID.values()).sort((a, b) =>
      a.timestamp.localeCompare(b.timestamp),
    );
  }, [history, liveActivities]);

  useEffect(() => {
    setLiveActivities([]);
    setStreamError(null);
  }, [agent?.id, sessionKey]);

  useEffect(() => {
    const spaceID = agent?.spaceId;
    if (!agent || !spaceID || !sessionKey) {
      setIsConnected(false);
      return;
    }

    let cancelled = false;
    let stopSubscription: (() => void) | null = null;
    let assistantDraft = "";
    const assistantID = `${sessionKey}:assistant:stream`;

    subscribeSessionEvents(
      spaceID,
      sessionKey,
      (event) => {
        if (cancelled) return;
        const text = eventText(event);
        if (text) {
          assistantDraft += text;
          upsertLiveActivity(
            assistantMessageActivity(sessionKey, assistantDraft, assistantID),
          );
          return;
        }
        const activity = sessionEventToActivity(event);
        if (activity) upsertLiveActivity(activity);
      },
      (error) => {
        if (cancelled) return;
        setIsConnected(false);
        setStreamError(error.message);
      },
    )
      .then((stop) => {
        if (cancelled) {
          stop();
          return;
        }
        stopSubscription = stop;
        setIsConnected(true);
      })
      .catch((error: Error) => {
        if (cancelled) return;
        setIsConnected(false);
        setStreamError(error.message);
      });

    return () => {
      cancelled = true;
      stopSubscription?.();
      setIsConnected(false);
    };

    function upsertLiveActivity(activity: ActivityRecord) {
      setLiveActivities((prev) => {
        const existing = prev.findIndex((item) => item.id === activity.id);
        if (existing === -1) return [...prev, activity];
        const next = [...prev];
        next[existing] = activity;
        return next;
      });
    }
  }, [agent, sessionKey]);

  const activities = useMemo(() => {
    if (!sessionKey) return allActivities;
    return allActivities.filter(
      (activity) =>
        activity.session_id === sessionKey ||
        activity.session_id === "" ||
        !activity.session_id,
    );
  }, [allActivities, sessionKey]);

  const send = useCallback(
    (message: string, mode: AgentMode = "ask", files?: FileAttachment[]) => {
      if (!agent || !sessionKey || !message.trim()) return;
      setLiveActivities((prev) => [
        ...prev,
        userMessageActivity(sessionKey, message),
      ]);
      sendMut.mutate(
        { message, mode, files, sessionKey },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({
              queryKey: activityKey(agent.id, sessionKey),
            });
          },
        },
      );
    },
    [agent, queryClient, sendMut, sessionKey],
  );

  return {
    activities,
    isSending: sendMut.isPending,
    isConnected,
    error: sendMut.error?.message ?? streamError,
    clearError: () => {
      sendMut.reset();
      setStreamError(null);
    },
    send,
  };
}
