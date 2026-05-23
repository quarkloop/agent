"use client";

import { cn } from "@/lib/utils";
import type { AgentConnection } from "@/lib/types";
import { StatusIndicator } from "@/components/layout/status-indicator";

interface AgentCardProps {
  agent: AgentConnection;
  active: boolean;
  onSelect: () => void;
}

export function AgentCard({ agent, active, onSelect }: AgentCardProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors",
        active
          ? "bg-foreground/[0.06] text-foreground"
          : "text-foreground/70 hover:bg-foreground/[0.04] hover:text-foreground",
      )}
    >
      <StatusIndicator status={agent.status} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{agent.name}</div>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="font-mono">{agent.spaceId ?? agent.id}</span>
          <span className="text-border">|</span>
          <span>{agent.mode}</span>
        </div>
      </div>
    </button>
  );
}
