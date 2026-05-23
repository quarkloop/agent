import { useMutation } from "@tanstack/react-query";
import { sendSessionMessage } from "@/lib/nats/client";
import type { AgentMode, FileAttachment, ChatResponse } from "@/lib/types";

export function useSendMessage(agentId: string | undefined, spaceId?: string) {
  return useMutation({
    mutationFn: async (vars: {
      message: string;
      mode?: AgentMode;
      files?: FileAttachment[];
      sessionKey?: string;
    }): Promise<ChatResponse> => {
      const { message, files, sessionKey } = vars;
      const resolvedSpace = spaceId ?? agentId;
      if (!resolvedSpace) throw new Error("space is required");
      if (!sessionKey) throw new Error("session is required");
      if (files && files.length > 0) {
        throw new Error(
          "File attachments require the NATS artifact contract before web upload can be enabled.",
        );
      }
      await sendSessionMessage({
        space_id: resolvedSpace,
        session_id: sessionKey,
        content: message,
      });
      return { reply: "" };
    },
    retry: false,
  });
}
