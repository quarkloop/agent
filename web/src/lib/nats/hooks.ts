"use client";

import { use, useEffect, useMemo, useState } from "react";
import type { NatsConnection, Status } from "@nats-io/nats-core";
import { controlConnection } from "@/lib/nats/connection";
import { requestPayload } from "@/lib/nats/envelope";

export type NatsConnectionStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "disconnected"
  | "closed"
  | "error";

export function useNatsConnection(): NatsConnection {
  return use(controlConnection());
}

export function useNatsStatus(connection: NatsConnection): {
  status: NatsConnectionStatus;
  detail: string;
} {
  const [state, setState] = useState({
    status: "connected" as NatsConnectionStatus,
    detail: connection.getServer(),
  });

  useEffect(() => {
    let cancelled = false;
    const statuses = connection.status();

    connection.closed().then((err) => {
      if (cancelled) return;
      setState({
        status: err ? "error" : "closed",
        detail: err ? err.message : "NATS connection closed",
      });
    });

    void (async () => {
      for await (const status of statuses) {
        if (cancelled) return;
        setState(statusToState(status, connection));
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [connection]);

  return state;
}

export function useNatsRequest<T>(
  subject: string,
  spaceID: string | undefined,
  payload: unknown,
): () => Promise<T> {
  const connection = useNatsConnection();
  return useMemo(
    () => () => requestPayload<T>(connection, subject, spaceID, payload),
    [connection, payload, spaceID, subject],
  );
}

export function useNatsSubscription<T>(
  subject: string | undefined,
  onMessage: (message: T) => void,
  onError?: (error: Error) => void,
) {
  const connection = useNatsConnection();

  useEffect(() => {
    if (!subject) return;
    const subscription = connection.subscribe(subject, {
      callback: (err, msg) => {
        if (err) {
          onError?.(err);
          return;
        }
        try {
          onMessage(msg.json<T>());
        } catch (error) {
          onError?.(error instanceof Error ? error : new Error(String(error)));
        }
      },
    });
    void connection.flush();
    return () => subscription.unsubscribe();
  }, [connection, onError, onMessage, subject]);
}

function statusToState(
  status: Status,
  connection: NatsConnection,
): { status: NatsConnectionStatus; detail: string } {
  switch (status.type) {
    case "disconnect":
      return { status: "disconnected", detail: status.server };
    case "reconnecting":
      return { status: "reconnecting", detail: "reconnecting" };
    case "reconnect":
      return { status: "connected", detail: status.server };
    case "error":
      return { status: "error", detail: status.error.message };
    case "close":
      return { status: "closed", detail: "NATS connection closed" };
    default:
      return { status: "connected", detail: connection.getServer() };
  }
}
