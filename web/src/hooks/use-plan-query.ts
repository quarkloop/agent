import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  approveRuntimePlan,
  rejectRuntimePlan,
  runtimePlan,
} from "@/lib/nats/client";
import type { Plan } from "@/lib/types";

const DISABLED_KEY = ["disabled"] as const;

function agentKey(agentId: string, path: string) {
  return ["agents", agentId, path] as const;
}

export function usePlan(agentId: string | undefined, spaceId?: string) {
  return useQuery<Plan | null>({
    queryKey: agentId ? agentKey(agentId, "plan") : DISABLED_KEY,
    queryFn: async () => {
      try {
        return await runtimePlan(spaceId ?? agentId!);
      } catch {
        return null;
      }
    },
    enabled: !!agentId && !!spaceId,
  });
}

export function useApprovePlan(agentId: string, spaceId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => approveRuntimePlan(spaceId ?? agentId),
    onSuccess(plan) {
      qc.setQueryData<Plan | null>(agentKey(agentId, "plan"), plan);
    },
  });
}

export function useRejectPlan(agentId: string, spaceId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => rejectRuntimePlan(spaceId ?? agentId),
    onSuccess() {
      qc.setQueryData<Plan | null>(agentKey(agentId, "plan"), null);
    },
  });
}
