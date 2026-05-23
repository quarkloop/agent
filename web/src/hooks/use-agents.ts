"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAgentContext } from "@/context/agent-context";
import {
  useAgents,
  useAddAgent,
  useRemoveAgent,
} from "@/hooks/use-agents-query";
import type { AgentConnection } from "@/lib/types";

export function useAgentsList() {
  const { state, dispatch, activeAgent } = useAgentContext();
  const discoveredOnce = useRef(false);

  const { data: discoveredAgents = [], refetch: queryRefetch } = useAgents();
  const addMut = useAddAgent();
  const removeMut = useRemoveAgent();

  const discover = useCallback(() => queryRefetch(), [queryRefetch]);

  // Sync discovered agents to context.
  useEffect(() => {
    if (discoveredAgents.length > 0 || discoveredOnce.current) {
      dispatch({ type: "DISCOVER_SUCCESS", agents: discoveredAgents });
      discoveredOnce.current = true;
    }
  }, [discoveredAgents, dispatch]);

  const addAgent = useCallback(
    (agent: AgentConnection) => {
      addMut.mutate(agent);
      dispatch({ type: "ADD_AGENT", agent });
    },
    [addMut, dispatch],
  );

  const removeAgent = useCallback(
    (id: string) => {
      removeMut.mutate(id);
      dispatch({ type: "REMOVE_AGENT", id });
    },
    [removeMut, dispatch],
  );

  const setActive = useCallback(
    (id: string | null) => {
      dispatch({ type: "SET_ACTIVE", id });
    },
    [dispatch],
  );

  return {
    agents: state.agents,
    activeAgent,
    isDiscovering: state.isDiscovering,
    discover,
    addAgent,
    removeAgent,
    setActive,
  };
}
