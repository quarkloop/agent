import { useQuery } from "@tanstack/react-query";
import { runtimeActivity } from "@/lib/nats/client";
import { runtimeActivityToUI } from "@/lib/nats/activity";
import type { ActivityRecord } from "@/lib/types";

const DISABLED_KEY = ["disabled"] as const;

export function activityKey(agentId: string, sessionKey?: string | null) {
  return [
    "agents",
    agentId,
    sessionKey ? `activity:${sessionKey}` : "activity",
  ] as const;
}

export function useActivity(
  agentId: string | undefined,
  sessionKey?: string | null,
  spaceId?: string,
) {
  return useQuery<ActivityRecord[]>({
    queryKey: agentId ? activityKey(agentId, sessionKey) : DISABLED_KEY,
    queryFn: async () => {
      const resp = await runtimeActivity(spaceId ?? agentId!, 128);
      const records = resp.records.map(runtimeActivityToUI);
      return sessionKey
        ? records.filter((record) => record.session_id === sessionKey)
        : records;
    },
    enabled: !!agentId && !!spaceId,
    staleTime: Infinity,
  });
}
