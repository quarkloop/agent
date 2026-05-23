"use client";

import { useState, useMemo } from "react";
import { cn } from "@/lib/utils";
import { Terminal, ChevronRight, Check, X, Loader2 } from "lucide-react";

interface ToolCallItemProps {
  toolName: string;
  args?: string;
  result?: string;
  isError?: boolean;
  timestamp: string;
}

export function ToolCallItem({
  toolName,
  args,
  result,
  isError,
}: ToolCallItemProps) {
  const [expanded, setExpanded] = useState(false);
  const completed = result !== undefined;
  const parsed = useMemo(
    () => parseToolData(toolName, args, result),
    [toolName, args, result],
  );

  return (
    <div className="px-5 py-0.5">
      {/* Compact inline pill — always visible */}
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className={cn(
          "inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-[12px] transition-colors",
          "border-zinc-200 bg-zinc-50 text-zinc-500 hover:border-zinc-300 hover:bg-zinc-100",
        )}
      >
        <ChevronRight
          className={cn(
            "size-3 text-zinc-400 transition-transform duration-150",
            expanded && "rotate-90",
          )}
        />
        <Terminal className="size-3 text-zinc-400" />
        <span className="font-mono font-medium text-zinc-600">{toolName}</span>
        {parsed.summary && (
          <>
            <span className="text-zinc-300">·</span>
            <span className="max-w-[300px] truncate font-mono text-zinc-400">
              {parsed.summary}
            </span>
          </>
        )}
        {isError ? (
          <X className="size-3 text-red-500" />
        ) : completed ? (
          <Check className="size-3 text-emerald-500" />
        ) : (
          <Loader2 className="size-3 animate-spin text-zinc-400" />
        )}
      </button>

      {/* Expanded: terminal + output */}
      {expanded && (
        <div className="ml-1 mt-1.5 mb-1 max-w-[600px] overflow-hidden rounded-lg border border-zinc-200">
          {/* Command bar */}
          {parsed.command && (
            <div className="bg-zinc-900 px-3 py-1.5 font-mono text-[12px] text-zinc-300">
              <span className="select-none text-zinc-500">$ </span>
              {parsed.command}
            </div>
          )}
          {/* Args fallback */}
          {!parsed.command && args && (
            <div className="bg-zinc-50 px-3 py-1.5 font-mono text-[12px] text-zinc-500">
              {truncate(args, 500)}
            </div>
          )}
          {/* Output */}
          {parsed.output && (
            <pre
              className={cn(
                "whitespace-pre-wrap break-all px-3 py-1.5 font-mono text-[12px]",
                isError ? "bg-red-50 text-red-600" : "bg-white text-zinc-600",
              )}
            >
              {truncate(parsed.output.trim(), 1500)}
            </pre>
          )}
          {/* Raw result fallback */}
          {!parsed.output && result && (
            <pre
              className={cn(
                "whitespace-pre-wrap break-all px-3 py-1.5 font-mono text-[12px]",
                isError ? "bg-red-50 text-red-600" : "bg-white text-zinc-600",
              )}
            >
              {truncate(result, 1500)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

interface ParsedToolData {
  summary: string;
  command?: string;
  output?: string;
}

function parseToolData(
  toolName: string,
  args?: string,
  result?: string,
): ParsedToolData {
  let command: string | undefined;
  let output: string | undefined;
  let summary = "";

  if (args) {
    try {
      const parsed = JSON.parse(args);
      if (toolName === "bash") {
        command = parsed.cmd || parsed.command || undefined;
        summary = command ? truncate(command, 50) : "";
      } else if (toolName === "read") {
        summary = parsed.path || "";
      } else if (toolName === "write") {
        const op = parsed.operation || "write";
        summary = `${op} ${parsed.path || ""}`.trim();
      } else {
        const entries = Object.entries(parsed);
        if (entries.length > 0) {
          summary = entries
            .map(([k, v]) => `${k}=${truncate(String(v), 20)}`)
            .join(" ");
        }
      }
    } catch {
      summary = truncate(args, 50);
    }
  }

  if (result) {
    try {
      const parsed = JSON.parse(result);
      output =
        parsed.output ?? parsed.content_preview ?? parsed.content ?? undefined;
      if (parsed.error && typeof parsed.error === "string") {
        output = parsed.error;
      }
    } catch {
      output = result;
    }
  }

  return { summary, command, output };
}

function truncate(s: string, max: number) {
  return s.length > max ? s.slice(0, max) + "\u2026" : s;
}
