import type {
  AppSchema,
  UploadResponse,
  VoiceClientEvent,
  VoiceServerEvent,
  VoiceSessionCallbacks,
  VoiceSessionConnection,
} from "@/types";

export async function loadSchema(): Promise<AppSchema> {
  const response = await fetch("/api/schema");
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  return response.json() as Promise<AppSchema>;
}

export async function predict(interfaceID: string, data: unknown[]): Promise<unknown[]> {
  const response = await fetch("/api/predict", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ interface_id: interfaceID, data }),
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  const payload = (await response.json()) as { data?: unknown[] };
  return payload.data ?? [];
}

export async function stream(
  interfaceID: string,
  data: unknown[],
  onChunk: (chunk: string) => void,
): Promise<void> {
  const response = await fetch("/api/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ interface_id: interfaceID, data }),
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
  if (!response.body) {
    throw new Error("stream response body is empty");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) {
      break;
    }

    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split("\n\n");
    buffer = events.pop() ?? "";
    for (const chunk of parseServerSentEvents(events.join("\n\n"))) {
      onChunk(chunk);
    }
  }

  buffer += decoder.decode();
  for (const chunk of parseServerSentEvents(buffer)) {
    onChunk(chunk);
  }
}

export async function uploadFile(file: File): Promise<UploadResponse> {
  const form = new FormData();
  form.append("file", file);

  const response = await fetch("/api/upload", {
    method: "POST",
    body: form,
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  return response.json() as Promise<UploadResponse>;
}

export function openVoiceSession(interfaceID: string, callbacks: VoiceSessionCallbacks): VoiceSessionConnection {
  const url = new URL(`/api/voice/${interfaceID}/ws`, window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";

  const socket = new WebSocket(url);
  const pending: VoiceClientEvent[] = [];

  socket.addEventListener("open", () => {
    for (const event of pending.splice(0)) {
      socket.send(JSON.stringify(event));
    }
  });

  socket.addEventListener("message", (event) => {
    try {
      callbacks.onEvent(JSON.parse(String(event.data)) as VoiceServerEvent);
    } catch (error) {
      callbacks.onError(error instanceof Error ? error : new Error(String(error)));
    }
  });

  socket.addEventListener("close", () => {
    callbacks.onClose();
  });

  socket.addEventListener("error", () => {
    callbacks.onError(new Error("voice session failed"));
  });

  return {
    send(event) {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify(event));
        return;
      }

      pending.push(event);
    },
    close() {
      socket.close();
    },
  };
}

export function parseServerSentEvents(buffer: string): string[] {
  return buffer
    .split("\n\n")
    .map((rawEvent) => rawEvent.trim())
    .filter(Boolean)
    .map(parseServerSentEvent)
    .filter((chunk): chunk is string => chunk !== null);
}

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const payload = (await response.json()) as { error?: { message?: string } };
    return payload.error?.message || response.statusText;
  } catch {
    return response.statusText;
  }
}

function parseServerSentEvent(rawEvent: string): string | null {
  const lines = rawEvent.split("\n");
  const event =
    lines
      .filter((line) => line.startsWith("event:"))
      .map((line) => line.slice("event:".length).trim())
      .at(-1) || "message";
  const data = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice("data:".length).trim())
    .join("\n")
    .replaceAll("\\n", "\n");

  if (event === "error") {
    throw new Error(data || "stream failed");
  }

  return data === "" ? null : data;
}
