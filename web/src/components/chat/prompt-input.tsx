"use client";

import { useState, useCallback } from "react";
import { PromptEditor } from "@/components/lexical/prompt-editor";
import { AttachmentPreview } from "./attachment-preview";
import { Paperclip, ArrowUp, ChevronDown } from "lucide-react";
import type { FileAttachment } from "@/lib/types";
import type { AgentMode } from "@/lib/types";

const MODES: { value: AgentMode; label: string; desc: string }[] = [
  { value: "ask", label: "Ask", desc: "Direct answer with optional tool use" },
  { value: "plan", label: "Plan", desc: "Execute a step-by-step plan" },
  { value: "masterplan", label: "Master Plan", desc: "Multi-phase project" },
  { value: "auto", label: "Auto", desc: "Agent decides the mode" },
];

interface PromptInputProps {
  onSend: (message: string, mode: AgentMode, files?: FileAttachment[]) => void;
  disabled?: boolean;
  model?: string;
  provider?: string;
}

export function PromptInput({
  onSend,
  disabled,
  model,
  provider,
}: PromptInputProps) {
  const [files, setFiles] = useState<FileAttachment[]>([]);
  const [mode, setMode] = useState<AgentMode>("ask");
  const [modeMenuOpen, setModeMenuOpen] = useState(false);

  const handleSubmit = useCallback(
    (text: string) => {
      if (!text.trim()) return;
      onSend(text, mode, files.length > 0 ? files : undefined);
      setFiles([]);
    },
    [onSend, mode, files],
  );

  const handleFilesAdded = useCallback((newFiles: File[]) => {
    const attachments: FileAttachment[] = newFiles.map((f) => ({
      name: f.name,
      mimeType: f.type || "application/octet-stream",
      size: f.size,
      file: f,
    }));
    setFiles((prev) => [...prev, ...attachments]);
  }, []);

  const handleRemoveFile = useCallback((index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const handleAttachClick = useCallback(() => {
    const input = document.createElement("input");
    input.type = "file";
    input.multiple = true;
    input.onchange = () => {
      if (input.files?.length) {
        handleFilesAdded(Array.from(input.files));
      }
    };
    input.click();
  }, [handleFilesAdded]);

  const currentMode = MODES.find((m) => m.value === mode)!;

  return (
    <div className="mx-auto w-full max-w-3xl px-4 pb-4 pt-2">
      <div className="rounded-2xl border border-border bg-muted/30 shadow-sm transition-colors focus-within:border-foreground/20">
        {/* Attachment previews */}
        {files.length > 0 && (
          <div className="px-3 pt-3">
            <AttachmentPreview files={files} onRemove={handleRemoveFile} />
          </div>
        )}

        {/* Editor area — full width */}
        <div className="px-3">
          <PromptEditor
            onSubmit={handleSubmit}
            onFilesAdded={handleFilesAdded}
            disabled={disabled}
            placeholder="Send a message…"
          />
        </div>

        {/* Bottom toolbar: attach, mode, model, send */}
        <div className="flex items-center px-2 pb-2">
          {/* Attach button */}
          <button
            type="button"
            onClick={handleAttachClick}
            disabled={disabled}
            aria-label="Attach file"
            className="flex size-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
          >
            <Paperclip className="size-4" />
          </button>

          {/* Mode selector */}
          <div className="relative">
            <button
              type="button"
              onClick={() => setModeMenuOpen(!modeMenuOpen)}
              className="flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <span className="font-medium">{currentMode.label}</span>
              <ChevronDown className="size-3" />
            </button>
            {modeMenuOpen && (
              <>
                <div
                  className="fixed inset-0 z-40"
                  onClick={() => setModeMenuOpen(false)}
                />
                <div className="absolute bottom-full left-0 z-50 mb-1 w-56 rounded-xl border border-border bg-background p-1 shadow-lg">
                  {MODES.map((m) => (
                    <button
                      key={m.value}
                      type="button"
                      onClick={() => {
                        setMode(m.value);
                        setModeMenuOpen(false);
                      }}
                      className={`flex w-full flex-col rounded-lg px-3 py-2 text-left transition-colors ${
                        mode === m.value
                          ? "bg-muted text-foreground"
                          : "text-foreground/70 hover:bg-muted/50"
                      }`}
                    >
                      <span className="text-sm font-medium">{m.label}</span>
                      <span className="text-xs text-muted-foreground">
                        {m.desc}
                      </span>
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>

          {/* Model badge — static display, not interactive */}
          {model && (
            <div className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs text-muted-foreground">
              {provider && (
                <span className="text-muted-foreground/60">{provider}/</span>
              )}
              <span className="font-medium">{model}</span>
            </div>
          )}

          {/* Spacer */}
          <div className="flex-1" />

          {/* Send button */}
          <button
            type="button"
            onClick={() => handleSubmit("")}
            disabled={disabled}
            aria-label="Send message"
            className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-foreground text-background transition-colors hover:bg-foreground/80 disabled:opacity-40"
          >
            <ArrowUp className="size-4" strokeWidth={2.5} />
          </button>
        </div>
      </div>
    </div>
  );
}
