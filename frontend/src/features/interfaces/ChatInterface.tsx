import { useRef, useState } from "react";
import { Send } from "lucide-react";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { ErrorAlert } from "@/components/ErrorAlert";
import { cancelRequest, streamWithEvents } from "@/lib/api";
import type { InterfaceSchema } from "@/types";
import { initialChatMessages, type ChatMessage, type DemoMode } from "@/features/demo/demo-mode";
import { errorMessage, normalizeStreamValue } from "@/features/schema/schema-utils";

export function ChatInterface({ iface, demoMode }: { iface: InterfaceSchema; demoMode: DemoMode }) {
  const [message, setMessage] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>(() => initialChatMessages(demoMode));
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [streamPhase, setStreamPhase] = useState<"idle" | "queued" | "running">("idle");
  const [streamRequestID, setStreamRequestID] = useState<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const baseMessages = useRef<ChatMessage[]>(initialChatMessages(demoMode));
  const input = iface.inputs[0] ?? { id: `${iface.id}-message`, type: "textbox", label: "Message", props: {} };

  async function submitMessage() {
    const trimmed = message.trim();
    if (trimmed === "" || isSubmitting) {
      return;
    }

    setError(null);
    setMessage("");
    setIsSubmitting(true);
    setStreamPhase("queued");
    setMessages((current) => [...current, { role: "user", content: trimmed }, { role: "assistant", content: "" }]);

    const controller = new AbortController();
    abortControllerRef.current = controller;
    setStreamRequestID(null);

    try {
      await streamWithEvents(
        iface.id,
        [trimmed],
        (event) => {
          if (event.status === "running") {
            setStreamPhase("running");
          }

          if (event.status === "queued") {
            setStreamPhase("queued");
          }

          if (event.status === "error") {
            setStreamPhase("idle");
            if (event.error) {
              setError(event.error);
            }
            return;
          }

          if (event.data === undefined) {
            return;
          }

          const chunk = normalizeStreamValue(event.data);
          if (chunk === "") {
            return;
          }

          setMessages((current) => {
            const next = [...current];
            const last = next[next.length - 1];
            if (last?.role === "assistant") {
              next[next.length - 1] = { ...last, content: last.content + chunk };
            }
            return next;
          });
        },
        {
          signal: controller.signal,
          onRequestID: setStreamRequestID,
        },
      );
    } catch (streamError) {
      if (streamError instanceof DOMException && streamError.name === "AbortError") {
        return;
      }

      setError(errorMessage(streamError));
      setStreamPhase("idle");
    } finally {
      setIsSubmitting(false);
      setStreamRequestID(null);
      setStreamPhase("idle");
    }
  }

  async function stopGeneration() {
    abortControllerRef.current?.abort();
    if (streamRequestID) {
      await cancelRequest(streamRequestID).catch((error) => {
        if (error instanceof Error) {
          setError(error.message);
        }
      });
      setStreamRequestID(null);
    }
  }

  function clearHistory() {
    setMessages(baseMessages.current);
    setError(null);
  }

  function copyHistory() {
    void navigator.clipboard.writeText(
      messages
        .map((entry) => `${entry.role === "user" ? "You" : "Assistant"}: ${entry.content}`)
        .join("\n"),
    );
  }

  function downloadHistory() {
    const payload = messages
      .map((entry) => `${entry.role === "user" ? "You" : "Assistant"}: ${entry.content}`)
      .join("\n");
    const blob = new Blob([payload], { type: "text/plain;charset=utf-8" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = `${iface.id}-chat.txt`;
    link.click();
    URL.revokeObjectURL(link.href);
  }

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Chat</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-6">
        <div
          aria-label="Chat transcript"
          role="log"
          className="flex min-h-72 flex-col gap-3 rounded-[1.25rem] border bg-muted/35 p-4"
        >
          {messages.length === 0 ? (
            <p className="text-sm leading-6 text-muted-foreground">Send a message to start the conversation.</p>
          ) : (
            messages.map((item, index) => (
              <div
                key={`${item.role}-${index}`}
                className={
                  item.role === "user"
                    ? "ml-auto max-w-[85%] rounded-3xl rounded-br-md bg-primary px-4 py-3 text-sm leading-6 text-primary-foreground"
                    : "mr-auto max-w-[85%] rounded-3xl rounded-bl-md border bg-card px-4 py-3 text-sm leading-6 shadow-sm"
                }
              >
                {item.content || (item.role === "assistant" && isSubmitting ? "..." : "")}
              </div>
            ))
          )}
        </div>
        {error ? <ErrorAlert title="Stream failed" message={error} /> : null}
        <Field>
          <FieldLabel htmlFor={input.id}>{input.label}</FieldLabel>
          <Textarea
            id={input.id}
            aria-label={input.label}
            disabled={isSubmitting}
            rows={3}
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                void submitMessage();
              }
            }}
          />
        </Field>
      </CardContent>
      <CardFooter className="grid gap-3 border-t bg-card/55 sm:grid-cols-[repeat(5,minmax(0,auto))]">
        <Button type="button" disabled={isSubmitting || message.trim() === ""} onClick={() => void submitMessage()}>
          {isSubmitting ? <Spinner /> : <Send data-icon="inline-start" />}
          {isSubmitting ? (streamPhase === "queued" ? "Queued" : "Running") : "Send"}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!isSubmitting}
          onClick={() => void stopGeneration()}
        >
          Stop
        </Button>
        <Button type="button" variant="outline" onClick={copyHistory}>
          Copy
        </Button>
        <Button type="button" variant="outline" onClick={downloadHistory}>
          Download
        </Button>
        <Button type="button" variant="secondary" onClick={clearHistory}>
          Clear history
        </Button>
      </CardFooter>
    </Card>
  );
}
