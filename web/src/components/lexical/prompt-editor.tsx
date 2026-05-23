"use client";

import { useCallback, useRef, useEffect } from "react";
import { LexicalComposer } from "@lexical/react/LexicalComposer";
import { RichTextPlugin } from "@lexical/react/LexicalRichTextPlugin";
import { ContentEditable } from "@lexical/react/LexicalContentEditable";
import { HistoryPlugin } from "@lexical/react/LexicalHistoryPlugin";
import { OnChangePlugin } from "@lexical/react/LexicalOnChangePlugin";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import {
  $getRoot,
  $createParagraphNode,
  type EditorState,
  type LexicalEditor,
} from "lexical";
import { editorTheme } from "./editor-theme";
import { SubmitPlugin } from "./plugins/submit-plugin";
import { AttachmentPlugin } from "./plugins/attachment-plugin";

interface PromptEditorProps {
  onSubmit: (text: string) => void;
  onFilesAdded: (files: File[]) => void;
  disabled?: boolean;
  placeholder?: string;
}

export function PromptEditor({
  onSubmit,
  onFilesAdded,
  disabled,
  placeholder = "Send a message\u2026",
}: PromptEditorProps) {
  const editorRef = useRef<LexicalEditor | null>(null);
  const textRef = useRef("");

  const handleChange = useCallback((editorState: EditorState) => {
    editorState.read(() => {
      textRef.current = $getRoot().getTextContent();
    });
  }, []);

  const handleSubmit = useCallback(() => {
    const text = textRef.current.trim();
    if (!text || disabled) return;
    onSubmit(text);
    editorRef.current?.update(() => {
      const root = $getRoot();
      root.clear();
      root.append($createParagraphNode());
    });
  }, [onSubmit, disabled]);

  const initialConfig = {
    namespace: "Prompt",
    theme: editorTheme,
    editable: !disabled,
    onError: (error: Error) => {
      console.error("Lexical prompt error:", error);
    },
  };

  return (
    <LexicalComposer initialConfig={initialConfig}>
      <div className="relative">
        <RichTextPlugin
          contentEditable={
            <ContentEditable
              className="min-h-[2.5rem] max-h-48 overflow-y-auto px-3 py-2.5 text-[15px] outline-none"
              aria-placeholder={placeholder}
              placeholder={
                <div className="pointer-events-none absolute top-0 left-0 px-3 py-2.5 text-sm text-muted-foreground select-none">
                  {placeholder}
                </div>
              }
            />
          }
          ErrorBoundary={LexicalErrorBoundary}
        />
        <HistoryPlugin />
        <OnChangePlugin onChange={handleChange} />
        <SubmitPlugin onSubmit={handleSubmit} />
        <AttachmentPlugin onFilesAdded={onFilesAdded} />
        <EditableStatePlugin disabled={disabled} />
        <EditorRefPlugin editorRef={editorRef} />
      </div>
    </LexicalComposer>
  );
}

function EditableStatePlugin({ disabled }: { disabled?: boolean }) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    editor.setEditable(!disabled);
  }, [disabled, editor]);
  return null;
}

function EditorRefPlugin({
  editorRef,
}: {
  editorRef: React.RefObject<LexicalEditor | null>;
}) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    editorRef.current = editor;
  }, [editor, editorRef]);
  return null;
}

function LexicalErrorBoundary({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
