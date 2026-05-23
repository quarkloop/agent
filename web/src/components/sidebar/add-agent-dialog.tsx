"use client";

import { useState } from "react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/themed/sheet";
import { Button } from "@/components/themed/button";
import { Input } from "@/components/themed/input";
import { Plus } from "lucide-react";
import type { AgentConnection } from "@/lib/types";
import { browserNatsConfig } from "@/lib/nats/config";

interface AddAgentDialogProps {
  onAdd: (agent: AgentConnection) => void;
}

export function AddAgentDialog({ onAdd }: AddAgentDialogProps) {
  const [open, setOpen] = useState(false);
  const [spaceId, setSpaceId] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const name = spaceId.trim();
    if (!name) return;
    const wsUrl = browserNatsConfig().wsUrl;
    const port = new URL(wsUrl).port;
    onAdd({
      id: name,
      name,
      mode: "direct",
      baseUrl: wsUrl,
      port: port ? Number(port) : 0,
      status: "unknown",
      spaceId: name,
    });
    setSpaceId("");
    setOpen(false);
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start gap-2"
          />
        }
      >
        <Plus className="size-4" />
        Add agent
      </SheetTrigger>
      <SheetContent side="left" className="w-72">
        <SheetHeader>
          <SheetTitle>Add Agent</SheetTitle>
        </SheetHeader>
        <form onSubmit={handleSubmit} className="mt-4 space-y-3">
          <Input
            type="text"
            placeholder="Space ID"
            value={spaceId}
            onChange={(e) => setSpaceId(e.target.value)}
          />
          <Button type="submit" size="sm" className="w-full">
            Connect
          </Button>
        </form>
      </SheetContent>
    </Sheet>
  );
}
