"use client";

import { createContext, useContext, type ReactNode } from "react";
import type { NatsConnectionStatus } from "@/lib/nats/hooks";

export interface NatsStatusState {
  status: NatsConnectionStatus;
  detail: string;
}

const NatsStatusContext = createContext<NatsStatusState>({
  status: "connecting",
  detail: "Connecting to NATS",
});

export function NatsStatusProvider({
  children,
  value,
}: {
  children: ReactNode;
  value: NatsStatusState;
}) {
  return <NatsStatusContext value={value}>{children}</NatsStatusContext>;
}

export function useNatsStatusState(): NatsStatusState {
  return useContext(NatsStatusContext);
}
