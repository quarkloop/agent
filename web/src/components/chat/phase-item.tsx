import { Badge } from "@/components/themed/badge";

interface PhaseItemProps {
  eventType: string;
  data?: Record<string, string>;
}

export function PhaseItem({ eventType, data }: PhaseItemProps) {
  const phaseId = data?.phase || "unknown";
  const variant =
    eventType === "phase.completed"
      ? "default"
      : eventType === "phase.failed"
        ? "destructive"
        : "outline";
  const label =
    eventType === "phase.started"
      ? "started"
      : eventType === "phase.completed"
        ? "completed"
        : "failed";

  return (
    <div className="mx-4 my-0.5 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground">
      <Badge variant={variant} className="h-4 px-1 text-[10px]">
        phase
      </Badge>
      <span>
        <span className="font-mono">{phaseId}</span> {label}
      </span>
      {data?.error && <span className="text-destructive">{data.error}</span>}
    </div>
  );
}
