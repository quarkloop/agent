interface SessionItemProps {
  eventType: string;
  data?: Record<string, string>;
  timestamp: string;
}

export function SessionItem({ eventType, data, timestamp }: SessionItemProps) {
  const time = formatTime(timestamp);
  const label =
    eventType === "session.started"
      ? `Session started${data?.agent ? ` \u00b7 ${data.agent}` : ""}`
      : `Session ended${data?.reason ? ` \u00b7 ${data.reason}` : ""}`;

  return (
    <div className="flex items-center justify-center py-4">
      <div className="flex items-center gap-2 text-[11px] text-zinc-400">
        <div className="h-px w-8 bg-zinc-200" />
        <span>
          {label} {time}
        </span>
        <div className="h-px w-8 bg-zinc-200" />
      </div>
    </div>
  );
}

function formatTime(ts: string) {
  try {
    return new Date(ts).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return "";
  }
}
