"use client";

import dynamic from "next/dynamic";
import type { ReactNode } from "react";
import { AgentProvider } from "@/context/agent-context";
import { Header } from "@/components/layout/header";

const NatsClientBoundary = dynamic<{ children: ReactNode }>(
  () =>
    import("../../components/nats/nats-client-boundary.js").then(
      (mod) => mod.default,
    ),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full flex-1 items-center justify-center text-sm text-muted-foreground">
        Connecting to NATS...
      </div>
    ),
  },
);

export default function ChatLayout({ children }: { children: ReactNode }) {
  return (
    <NatsClientBoundary>
      <AgentProvider>
        <div className="flex h-full flex-col">
          <Header />
          <div className="flex min-h-0 flex-1">{children}</div>
        </div>
      </AgentProvider>
    </NatsClientBoundary>
  );
}
