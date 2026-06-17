import type { AppSchema, UploadResponse } from "@/types";

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
