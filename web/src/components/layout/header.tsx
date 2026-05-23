"use client";

import { useAgentContext } from "@/context/agent-context";
import { StatusIndicator } from "./status-indicator";
import { useNatsStatusState } from "@/lib/nats/status-context";
import { Radio, Zap } from "lucide-react";

export function Header() {
  const { activeAgent } = useAgentContext();
  const nats = useNatsStatusState();

  return (
    <header className="flex h-14 shrink-0 items-center border-b border-border/60 bg-background px-5">
      <div className="flex items-center gap-2.5">
        <div className="flex size-7 items-center justify-center rounded-lg bg-foreground">
          <Zap className="size-3.5 text-background" strokeWidth={2.5} />
        </div>
        <span className="text-base font-semibold tracking-tight">Quark</span>
      </div>

      {activeAgent && (
        <>
          <div className="mx-4 h-4 w-px bg-border" />
          <div className="flex items-center gap-2 text-sm">
            <StatusIndicator status={activeAgent.status} />
            <span className="font-medium text-foreground/80">
              {activeAgent.name}
            </span>
            <span className="font-mono text-sm text-muted-foreground">
              {activeAgent.spaceId ?? activeAgent.id}
            </span>
          </div>
        </>
      )}

      <div className="ml-auto flex items-center gap-2 rounded-md border border-border/60 px-2 py-1 text-xs text-muted-foreground">
        <Radio className="size-3" />
        <span className="capitalize">{nats.status}</span>
        <span className="max-w-40 truncate font-mono text-[11px]">
          {nats.detail}
        </span>
      </div>
    </header>
  );
}
