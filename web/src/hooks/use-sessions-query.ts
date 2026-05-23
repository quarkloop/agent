import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createSession, deleteSession, listSessions } from "@/lib/nats/client";
import type {
  SessionRecord,
  CreateSessionRequest,
  CreateSessionResponse,
} from "@/lib/types";

const DISABLED_KEY = ["disabled"] as const;

function sessionsKey(agentId: string) {
  return ["agents", agentId, "sessions"] as const;
}

export function useSessions(agentId: string | undefined, spaceId?: string) {
  return useQuery<SessionRecord[]>({
    queryKey: agentId ? sessionsKey(agentId) : DISABLED_KEY,
    queryFn: () => listSessions(spaceId ?? agentId!),
    enabled: !!agentId && !!spaceId,
  });
}

export function useCreateSession(agentId: string, spaceId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: CreateSessionRequest): Promise<CreateSessionResponse> =>
      createSession(spaceId ?? agentId, req),
    onSuccess(resp) {
      if (!resp.session) return;
      qc.setQueryData<SessionRecord[]>(sessionsKey(agentId), (prev) =>
        prev ? [...prev, resp.session!] : [resp.session!],
      );
    },
  });
}

export function useDeleteSession(agentId: string, spaceId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionKey: string) =>
      deleteSession(spaceId ?? agentId, sessionKey),
    onSuccess(_, sessionKey) {
      qc.setQueryData<SessionRecord[]>(sessionsKey(agentId), (prev) =>
        prev ? prev.filter((s) => s.key !== sessionKey) : [],
      );
    },
  });
}
