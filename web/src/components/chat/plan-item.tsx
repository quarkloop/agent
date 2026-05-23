"use client";

import { useState } from "react";
import { ClipboardList, ArrowRight, Check, X, Loader2 } from "lucide-react";
import { useAgentContext } from "@/context/agent-context";
import { useApprovePlan, useRejectPlan, usePlan } from "@/hooks/use-plan-query";

interface PlanItemProps {
  eventType: string;
  data?: Record<string, string>;
  timestamp: string;
}

const stepIcons: Record<string, React.ReactNode> = {
  "step.dispatched": <ArrowRight className="size-3 text-blue-500" />,
  "step.completed": <Check className="size-3 text-emerald-500" />,
  "step.failed": <X className="size-3 text-destructive" />,
};

export function PlanItem({ eventType, data }: PlanItemProps) {
  const { activeAgent } = useAgentContext();
  const [acted, setActed] = useState<"approved" | "rejected" | null>(null);

  const agentId = activeAgent?.id;
  const spaceId = activeAgent?.spaceId;
  const approveMut = useApprovePlan(agentId ?? "", spaceId);
  const rejectMut = useRejectPlan(agentId ?? "", spaceId);
  const { data: plan } = usePlan(agentId, spaceId);

  const handleApprove = () => {
    if (!activeAgent || approveMut.isPending) return;
    approveMut.mutate(undefined, { onSuccess: () => setActed("approved") });
  };

  const handleReject = () => {
    if (!activeAgent || rejectMut.isPending) return;
    rejectMut.mutate(undefined, { onSuccess: () => setActed("rejected") });
  };

  if (eventType === "plan.created" || eventType === "masterplan.created") {
    const isDraft = !data?.status || data.status === "draft";
    const showActions = isDraft && !acted && activeAgent;

    return (
      <div className="mx-4 my-1 rounded-md border border-border bg-muted/50 px-3 py-2 text-sm">
        <div className="flex items-center gap-2">
          <ClipboardList className="size-3 text-muted-foreground" />
          <span className="font-medium">
            {eventType === "masterplan.created" ? "Master plan" : "Plan"}{" "}
            created
          </span>
          {data?.goal && (
            <span className="text-muted-foreground">— {data.goal}</span>
          )}
        </div>
        {acted && (
          <div className="mt-1 text-xs text-muted-foreground">Plan {acted}</div>
        )}
        {plan && plan.steps.length > 0 && (
          <div className="mt-2 space-y-1">
            {plan.steps.map((step) => (
              <div
                key={step.id}
                className="flex items-center gap-2 text-xs text-muted-foreground"
              >
                {stepIcons[`step.${step.status}`] || (
                  <Loader2 className="size-3 animate-spin text-muted-foreground" />
                )}
                <span className="font-mono">{step.id}</span>
                <span className="truncate">{step.description}</span>
              </div>
            ))}
          </div>
        )}
        {showActions && (
          <div className="mt-2 flex gap-2">
            <button
              onClick={handleApprove}
              disabled={approveMut.isPending}
              className="rounded-md bg-emerald-50 px-3 py-1 text-xs font-medium text-emerald-700 hover:bg-emerald-100 disabled:opacity-50 dark:bg-emerald-950 dark:text-emerald-300 dark:hover:bg-emerald-900"
            >
              Approve
            </button>
            <button
              onClick={handleReject}
              disabled={rejectMut.isPending}
              className="rounded-md bg-red-50 px-3 py-1 text-xs font-medium text-red-700 hover:bg-red-100 disabled:opacity-50 dark:bg-red-950 dark:text-red-300 dark:hover:bg-red-900"
            >
              Reject
            </button>
          </div>
        )}
      </div>
    );
  }

  if (eventType === "plan.updated" && data?.status) {
    return (
      <div className="mx-4 my-0.5 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground">
        <ClipboardList className="size-3 text-muted-foreground" />
        <span>
          Plan {data.status}
          {data.goal && <span> — {data.goal}</span>}
        </span>
      </div>
    );
  }

  const stepId = data?.step || "unknown";
  const icon = stepIcons[eventType] || (
    <Loader2 className="size-3 animate-spin text-muted-foreground" />
  );
  const label =
    eventType === "step.dispatched"
      ? "dispatched"
      : eventType === "step.completed"
        ? "completed"
        : "failed";

  return (
    <div className="mx-4 my-0.5 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground">
      {icon}
      <span>
        Step <span className="font-mono">{stepId}</span> {label}
      </span>
      {data?.error && <span className="text-destructive">{data.error}</span>}
    </div>
  );
}
