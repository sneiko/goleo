import type {
  AppSchema,
  EventResponseData,
  UploadResponse,
  StreamEvent,
  VoiceClientEvent,
  VoiceServerEvent,
  VoiceSessionCallbacks,
  VoiceSessionConnection,
} from "@/types";

const requestIDHeader = "X-Request-ID";

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

export type EventOptions = {
  requestID?: string;
};

export async function sendEvent(
  interfaceID: string,
  eventID: string,
  data: Record<string, unknown>,
  options?: EventOptions,
): Promise<EventResponseData> {
  const body: {
    interface_id: string;
    event_id: string;
    data: Record<string, unknown>;
    request_id?: string;
  } = {
    interface_id: interfaceID,
    event_id: eventID,
    data,
  };
  if (options?.requestID !== undefined) {
    body.request_id = options.requestID;
  }

  const response = await fetch("/api/event", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  const payload = (await response.json()) as { data?: EventResponseData };
  return payload.data ?? {};
}

export async function stream(
  interfaceID: string,
  data: unknown[],
  onChunk: (chunk: string) => void,
): Promise<void> {
  await streamWithEvents(interfaceID, data, (event) => {
    if (event.data !== undefined) {
      onChunk(normalizeStreamChunk(event.data));
    }
  });
}

export async function cancelRequest(requestID: string): Promise<void> {
  const response = await fetch("/api/cancel", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ request_id: requestID }),
  });

  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
}

export type StreamOptions = {
  signal?: AbortSignal;
  onRequestID?: (requestID: string) => void;
};

export async function streamWithEvents(
  interfaceID: string,
  data: unknown[],
  onEvent: (event: StreamEvent) => void,
  options?: StreamOptions,
): Promise<void> {
  const signal = options?.signal;
  const onRequestID = options?.onRequestID;

  const response = await fetch("/api/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ interface_id: interfaceID, data }),
    signal,
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  const requestID = response.headers.get(requestIDHeader);
  if (requestID) {
    onRequestID?.(requestID);
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
    for (const event of parseServerSentEventsWithTypes(events.join("\n\n"))) {
      onEvent(event);
    }
  }

  buffer += decoder.decode();
  for (const event of parseServerSentEventsWithTypes(buffer)) {
    onEvent(event);
  }
}

function normalizeStreamChunk(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }

  return JSON.stringify(value);
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
  return parseServerSentEventsWithTypes(buffer)
    .filter((event) => event.event === "" || event.event === "message" || event.event === "data")
    .map((event) => normalizeStreamChunk(event.data));
}

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const payload = (await response.json()) as { error?: { message?: string } };
    return payload.error?.message || response.statusText;
  } catch {
    return response.statusText;
  }
}

export function parseServerSentEventsWithTypes(buffer: string): StreamEvent[] {
  return buffer
    .split("\n\n")
    .map((rawEvent) => rawEvent.trim())
    .filter(Boolean)
    .map(parseServerSentEvent)
    .filter((event): event is StreamEvent => event !== null);
}

function parseServerSentEvent(rawEvent: string): StreamEvent | null {
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

  if (!data) {
    return null;
  }

  if (event === "error") {
    return {
      event,
      status: "error",
      error: data,
    };
  }

  let decoded: { [key: string]: unknown } | null = null;
  try {
    decoded = JSON.parse(data);
  } catch {
    return {
      event,
      status: event,
      data,
    };
  }

  const result: StreamEvent = { event, status: event };
  if (typeof decoded === "object" && decoded !== null) {
    if (typeof (decoded as { status?: unknown }).status === "string") {
      result.status = (decoded as { status?: string }).status;
    }
    if ("data" in decoded) {
      result.data = (decoded as { data?: unknown }).data;
    }
    if ("error" in decoded) {
      result.error = (decoded as { error?: string }).error;
    }
    if ("request_id" in decoded) {
      result.request_id = (decoded as { request_id?: string }).request_id;
    }
    if ("progress" in decoded) {
      result.progress = (decoded as { progress?: StreamEvent["progress"] }).progress;
    }

    if (result.data === undefined) {
      result.data = data;
    }
    return result;
  }

  return {
    event,
    status: event,
    data,
  };
}
