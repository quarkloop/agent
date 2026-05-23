"use client";

import { Suspense, type ReactNode } from "react";
import { Radio } from "lucide-react";
import { useNatsConnection, useNatsStatus } from "@/lib/nats/hooks";
import { NatsStatusProvider } from "@/lib/nats/status-context";

export default function NatsClientBoundary({
  children,
}: {
  children: ReactNode;
}) {
  return (
    <Suspense fallback={<NatsFallback />}>
      <NatsRuntime>{children}</NatsRuntime>
    </Suspense>
  );
}

function NatsRuntime({ children }: { children: ReactNode }) {
  const connection = useNatsConnection();
  const status = useNatsStatus(connection);

  return <NatsStatusProvider value={status}>{children}</NatsStatusProvider>;
}

function NatsFallback() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <div className="flex size-10 items-center justify-center rounded-xl bg-muted">
        <Radio className="size-4 animate-pulse text-muted-foreground" />
      </div>
      <p className="text-sm text-muted-foreground">Connecting to NATS...</p>
    </div>
  );
}
