import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { listAgents } from "@/lib/nats/client";
import type { AgentConnection } from "@/lib/types";

export const AGENTS_KEY = ["agents"] as const;

export function useAgents() {
  return useQuery<AgentConnection[]>({
    queryKey: AGENTS_KEY,
    queryFn: listAgents,
  });
}

export function useAddAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (agent: AgentConnection) => agent,
    onSuccess(agent) {
      qc.setQueryData<AgentConnection[]>(AGENTS_KEY, (prev) => {
        if (!prev) return [agent];
        if (prev.some((a) => a.id === agent.id)) return prev;
        return [...prev, agent];
      });
    },
  });
}

export function useRemoveAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => id,
    onSuccess(id) {
      qc.setQueryData<AgentConnection[]>(AGENTS_KEY, (prev) =>
        prev ? prev.filter((a) => a.id !== id) : [],
      );
    },
  });
}
