import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import * as api from "./lib/api";
import type { AppSchema } from "./types";

vi.mock("./lib/api");

const mockedAPI = vi.mocked(api);

describe("Goleo frontend", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    window.history.replaceState({}, "", "/");
  });

  it("shows an empty state when no interfaces are registered", async () => {
    mockedAPI.loadSchema.mockResolvedValue({
      version: "0.1.0",
      interfaces: [],
    });

    render(<App />);

    expect(await screen.findByText("No interfaces registered")).toBeInTheDocument();
    expect(screen.getByText("Add an Interface or Chat in Go to render controls here.")).toBeInTheDocument();
  });

  it("submits interface values and disables the run button while loading", async () => {
    mockedAPI.loadSchema.mockResolvedValue(interfaceSchema());
    const prediction = deferred<unknown[]>();
    mockedAPI.predict.mockReturnValue(prediction.promise);
    const user = userEvent.setup();

    render(<App />);

    await user.type(await screen.findByLabelText("Prompt"), "Ada");
    await user.click(screen.getByRole("button", { name: /run/i }));

    expect(screen.getByRole("button", { name: /running/i })).toBeDisabled();
    expect(mockedAPI.predict).toHaveBeenCalledWith("interface-1", ["Ada"]);
    prediction.resolve(["Hello Ada"]);
    expect(await screen.findByText("Hello Ada")).toBeInTheDocument();
  });

  it("keeps streamed chat messages in a transcript", async () => {
    mockedAPI.loadSchema.mockResolvedValue(chatSchema());
    mockedAPI.streamWithEvents.mockImplementation(async (_id, _data, onEvent) => {
      onEvent({ event: "running", status: "running", data: "Hel" });
      onEvent({ event: "data", status: "running", data: "lo" });
      onEvent({ event: "done", status: "done" });
    });
    const user = userEvent.setup();

    render(<App />);

    await user.type(await screen.findByLabelText("Message"), "Hi{Enter}");

    expect(mockedAPI.streamWithEvents).toHaveBeenCalledWith(
      "chat-1",
      ["Hi"],
      expect.any(Function),
      expect.objectContaining({
        signal: expect.any(AbortSignal),
      }),
    );
    expect(await screen.findByText("Hi")).toBeInTheDocument();
    expect(await screen.findByText("Hello")).toBeInTheDocument();

    const transcript = screen.getByRole("log", { name: "Chat transcript" });
    expect(within(transcript).getAllByText(/Hi|Hello/)).toHaveLength(2);
  });

  it("renders uploaded file metadata before submission", async () => {
    mockedAPI.loadSchema.mockResolvedValue(fileSchema());
    mockedAPI.uploadFile.mockResolvedValue({
      id: "upload-prompt.txt-5",
      name: "prompt.txt",
      size: 5,
      content_type: "text/plain",
    });
    const user = userEvent.setup();

    render(<App />);

    const file = new File(["hello"], "prompt.txt", { type: "text/plain" });
    await user.upload(await screen.findByLabelText("Document"), file);

    expect(mockedAPI.uploadFile).toHaveBeenCalledWith(file);
    expect(await screen.findByText("prompt.txt")).toBeInTheDocument();
    expect(screen.getByText("5 B text/plain")).toBeInTheDocument();
  });

  it("renders audio inputs as media controls instead of a textarea", async () => {
    mockedAPI.loadSchema.mockResolvedValue(audioSchema());
    mockedAPI.uploadFile.mockResolvedValue({
      id: "asset-audio-1",
      name: "prompt.wav",
      size: 5,
      content_type: "audio/wav",
      url: "/api/assets/asset-audio-1",
    });
    const user = userEvent.setup();

    render(<App />);

    expect(await screen.findByRole("button", { name: "Record" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Prompt audio" })).not.toBeInTheDocument();

    const file = new File(["hello"], "prompt.wav", { type: "audio/wav" });
    await user.upload(screen.getByLabelText("Prompt audio"), file);

    expect(mockedAPI.uploadFile).toHaveBeenCalledWith(file);
    expect(await screen.findByLabelText("Prompt audio preview")).toHaveAttribute("src", "/api/assets/asset-audio-1");
  });

  it("records audio from the microphone and uploads the captured blob", async () => {
    mockedAPI.loadSchema.mockResolvedValue(audioSchema());
    mockedAPI.uploadFile.mockResolvedValue({
      id: "asset-audio-2",
      name: "recording.webm",
      size: 9,
      content_type: "audio/webm",
      url: "/api/assets/asset-audio-2",
    });

    const mediaStream = { getTracks: () => [{ stop: vi.fn() }] } as unknown as MediaStream;
    const getUserMedia = vi.fn().mockResolvedValue(mediaStream);
    vi.stubGlobal("navigator", {
      mediaDevices: {
        getUserMedia,
      },
    });

    class MockMediaRecorder {
      static instances: MockMediaRecorder[] = [];

      ondataavailable: ((event: BlobEvent) => void) | null = null;
      onstop: (() => void) | null = null;
      mimeType = "audio/webm";
      stream: MediaStream;

      constructor(stream: MediaStream) {
        this.stream = stream;
        MockMediaRecorder.instances.push(this);
      }

      start() {}

      stop() {
        const blob = new Blob(["voice-data"], { type: this.mimeType });
        this.ondataavailable?.({ data: blob } as BlobEvent);
        this.onstop?.();
      }
    }

    vi.stubGlobal("MediaRecorder", MockMediaRecorder as unknown as typeof MediaRecorder);

    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Record" }));
    expect(getUserMedia).toHaveBeenCalledWith({ audio: true });

    await user.click(screen.getByRole("button", { name: /stop recording/i }));
    expect(mockedAPI.uploadFile).toHaveBeenCalledTimes(1);
    expect(await screen.findByLabelText("Prompt audio preview")).toHaveAttribute("src", "/api/assets/asset-audio-2");
  });

  it("renders audio outputs as playable media blocks", async () => {
    mockedAPI.loadSchema.mockResolvedValue(audioSchema());
    mockedAPI.predict.mockResolvedValue([
      {
        id: "asset-reply-1",
        name: "reply.wav",
        size: 5,
        content_type: "audio/wav",
        url: "/api/assets/asset-reply-1",
      },
    ]);
    const user = userEvent.setup();

    render(<App />);

    const file = new File(["hello"], "prompt.wav", { type: "audio/wav" });
    mockedAPI.uploadFile.mockResolvedValue({
      id: "asset-audio-1",
      name: "prompt.wav",
      size: 5,
      content_type: "audio/wav",
      url: "/api/assets/asset-audio-1",
    });
    await user.upload(await screen.findByLabelText("Prompt audio"), file);
    await user.click(screen.getByRole("button", { name: /run/i }));

    expect(await screen.findByLabelText("Reply audio preview")).toHaveAttribute("src", "/api/assets/asset-reply-1");
  });

  it("renders a voice session shell and reacts to text and audio websocket events", async () => {
    mockedAPI.loadSchema.mockResolvedValue(voiceSchema());
    const send = vi.fn();
    const close = vi.fn();
    let callbacks!: Parameters<typeof api.openVoiceSession>[1];
    mockedAPI.openVoiceSession.mockImplementation((_, next) => {
      callbacks = next;
      return { send, close };
    });
    const user = userEvent.setup();

    render(<App />);

    await user.click(await screen.findByRole("button", { name: /connect voice/i }));

    expect(mockedAPI.openVoiceSession).toHaveBeenCalledWith("voice-1", expect.any(Object));
    expect(send).toHaveBeenCalledWith({ type: "session.start" });

    callbacks.onEvent({ type: "session.ready" });
    callbacks.onEvent({ type: "output.text", text: "heard hello" });
    callbacks.onEvent({
      type: "output.audio",
      audio: {
        id: "asset-reply-voice-1",
        name: "reply.wav",
        size: 5,
        content_type: "audio/wav",
        url: "/api/assets/asset-reply-voice-1",
      },
    });

    expect(await screen.findByText("heard hello")).toBeInTheDocument();
    expect(screen.getByLabelText("Voice reply preview")).toHaveAttribute("src", "/api/assets/asset-reply-voice-1");

    await user.click(screen.getByRole("button", { name: /interrupt/i }));
    expect(send).toHaveBeenCalledWith({ type: "output.interrupt" });

    callbacks.onEvent({ type: "session.closed" });
    expect(await screen.findByText("Voice session closed.")).toBeInTheDocument();
    expect(close).toHaveBeenCalledTimes(1);
  });

  it("streams microphone chunks to the voice session and sends input.stop on mute", async () => {
    mockedAPI.loadSchema.mockResolvedValue(voiceSchema());
    const send = vi.fn();
    let callbacks!: Parameters<typeof api.openVoiceSession>[1];
    mockedAPI.openVoiceSession.mockImplementation((_, next) => {
      callbacks = next;
      return { send, close: vi.fn() };
    });

    const mediaStream = { getTracks: () => [{ stop: vi.fn() }] } as unknown as MediaStream;
    const getUserMedia = vi.fn().mockResolvedValue(mediaStream);
    vi.stubGlobal("navigator", {
      mediaDevices: {
        getUserMedia,
      },
    });

    class MockMediaRecorder {
      static instances: MockMediaRecorder[] = [];

      ondataavailable: ((event: BlobEvent) => void) | null = null;
      onstop: (() => void) | null = null;
      mimeType = "audio/webm";
      stream: MediaStream;

      constructor(stream: MediaStream) {
        this.stream = stream;
        MockMediaRecorder.instances.push(this);
      }

      start() {}

      emitChunk(blob: Blob) {
        this.ondataavailable?.({ data: blob } as BlobEvent);
      }

      stop() {
        this.onstop?.();
      }
    }

    vi.stubGlobal("MediaRecorder", MockMediaRecorder as unknown as typeof MediaRecorder);

    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: /connect voice/i }));
    callbacks.onEvent({ type: "session.ready" });

    await user.click(screen.getByRole("button", { name: "Unmute mic" }));
    expect(getUserMedia).toHaveBeenCalledWith({ audio: true });

    MockMediaRecorder.instances[0]?.emitChunk({
      size: 10,
      arrayBuffer: vi.fn().mockResolvedValue(new TextEncoder().encode("voice-data").buffer),
    } as unknown as Blob);
    await waitFor(() =>
      expect(send).toHaveBeenCalledWith({
        type: "input.audio",
        audio: {
          mime_type: "audio/webm",
          sequence: 1,
          data: btoa("voice-data"),
        },
      }),
    );

    await user.click(screen.getByRole("button", { name: "Mute mic" }));
    await waitFor(() => expect(send).toHaveBeenCalledWith({ type: "input.stop" }));

    await user.click(screen.getByRole("button", { name: /disconnect/i }));
    expect(send).toHaveBeenCalledWith({ type: "session.close" });
  });

  it("renders seeded showcase outputs and uploaded file metadata in readme demo mode", async () => {
    window.history.replaceState({}, "", "/?demo=readme-hero");
    mockedAPI.loadSchema.mockResolvedValue(showcaseFormSchema());

    render(<App />);

    expect(await screen.findByText("Launch summary")).toBeInTheDocument();
    expect(screen.getByText("support-brief.md")).toBeInTheDocument();
    expect(screen.getByText(/Reduce repetitive ticket handling time/i)).toBeInTheDocument();
    expect(screen.getByText(/"audience": "Product leaders"/)).toBeInTheDocument();
  });

  it("renders a seeded chat transcript in readme chat mode", async () => {
    window.history.replaceState({}, "", "/?demo=readme-chat");
    mockedAPI.loadSchema.mockResolvedValue(chatSchema());

    render(<App />);

    expect(await screen.findByText("Need a cleaner launch message for an internal support copilot.")).toBeInTheDocument();
    expect(screen.getByText(/Start with the outcome your team cares about/i)).toBeInTheDocument();
  });

  it("shows richer adapter metadata blocks in readme adapter mode", async () => {
    window.history.replaceState({}, "", "/?demo=readme-adapters");
    mockedAPI.loadSchema.mockResolvedValue(showcaseAdapterSchema());

    render(<App />);

    expect(await screen.findByText("Backend metadata")).toBeInTheDocument();
    expect(screen.getByText(/normalized handler binding/i)).toBeInTheDocument();
    expect(screen.getAllByText(/HTTP adapter/i).length).toBeGreaterThan(0);
  });

  it("renders blocks components and sends click event payloads before applying output values", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksClickSchema());
    mockedAPI.sendEvent.mockResolvedValue({ result: "Hello Ada" });
    const user = userEvent.setup();

    render(<App />);

    await user.clear(await screen.findByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Ada");
    await user.click(screen.getByRole("button", { name: "Generate" }));

    expect(mockedAPI.sendEvent).toHaveBeenCalledWith("blocks-1", "generate-click", { name: "Ada" });
    expect(await screen.findByText("Hello Ada")).toBeInTheDocument();
  });

  it("applies blocks update envelopes to values and target button runtime state", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksClickSchema());
    mockedAPI.sendEvent.mockResolvedValue({
      result: { __goleo_update__: true, kind: "update", value: "Done" },
      generate: { __goleo_update__: true, kind: "update", label: "Generated", disabled: true },
    });
    const user = userEvent.setup();

    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Generate" }));

    expect(await screen.findByText("Done")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Generated" })).toBeDisabled();
  });

  it("renders unmarked kind update objects as ordinary blocks output values", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksUpdateCollisionSchema());
    mockedAPI.sendEvent.mockResolvedValue({ result: { kind: "update", status: "ok" } });
    const user = userEvent.setup();

    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Check" }));

    expect(await screen.findByText(/"kind": "update"/)).toBeInTheDocument();
    expect(screen.getByText(/"status": "ok"/)).toBeInTheDocument();
  });

  it("runs blocks load events on mount and applies the response", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksLoadSchema());
    mockedAPI.sendEvent.mockResolvedValue({ status: "Loaded from server" });

    render(<App />);

    await waitFor(() => expect(mockedAPI.sendEvent).toHaveBeenCalledWith("blocks-load", "load-status", {}));
    expect(await screen.findByText("Loaded from server")).toBeInTheDocument();
  });

  it("only runs load-trigger blocks events on mount", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksLoadStrictSchema());
    mockedAPI.sendEvent.mockResolvedValue({ status: "Loaded once" });

    render(<App />);

    expect(await screen.findByText("Loaded once")).toBeInTheDocument();
    expect(mockedAPI.sendEvent).toHaveBeenCalledTimes(1);
    expect(mockedAPI.sendEvent).toHaveBeenCalledWith("blocks-load-strict", "load-status", {});
  });

  it("dispatches blocks change events with component values", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksChangeSchema());
    mockedAPI.sendEvent.mockResolvedValue({ preview: "Preview: abc" });
    const user = userEvent.setup();

    render(<App />);

    await user.type(await screen.findByLabelText("Prompt"), "abc");

    await waitFor(() =>
      expect(mockedAPI.sendEvent).toHaveBeenLastCalledWith("blocks-change", "prompt-change", {
        prompt: "abc",
      }),
    );
    expect(await screen.findByText("Preview: abc")).toBeInTheDocument();
  });

  it("keeps a blocks source disabled until all concurrent events from that source finish", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksConcurrentClickSchema());
    const first = deferred<Record<string, unknown>>();
    const second = deferred<Record<string, unknown>>();
    mockedAPI.sendEvent.mockImplementation((_interfaceID, eventID) => {
      if (eventID === "first-click") {
        return first.promise;
      }
      if (eventID === "second-click") {
        return second.promise;
      }
      return Promise.resolve({});
    });
    const user = userEvent.setup();

    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Run both" }));
    expect(screen.getByRole("button", { name: /run both/i })).toBeDisabled();
    await waitFor(() => expect(mockedAPI.sendEvent).toHaveBeenCalledTimes(2));

    first.resolve({ firstResult: "First done" });
    expect(await screen.findByText("First done")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /run both/i })).toBeDisabled();

    second.resolve({ secondResult: "Second done" });
    expect(await screen.findByText("Second done")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run both" })).toBeEnabled();
  });

  it("excludes inputs hidden by ancestor layout props from blocks event payloads", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksHiddenGroupSchema());
    mockedAPI.sendEvent.mockResolvedValue({});
    const user = userEvent.setup();

    render(<App />);

    expect(await screen.findByLabelText("Visible prompt")).toBeInTheDocument();
    expect(screen.queryByLabelText("Hidden prompt")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Send visible" }));

    expect(mockedAPI.sendEvent).toHaveBeenCalledWith(
      "blocks-hidden-group",
      "send-click",
      {
        visiblePrompt: "public",
      },
      { hidden: ["hiddenPrompt"] },
    );
  });

  it("excludes inputs hidden by runtime ancestor layout updates from later blocks event payloads", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksRuntimeHiddenGroupSchema());
    mockedAPI.sendEvent
      .mockResolvedValueOnce({ advanced: { __goleo_update__: true, kind: "update", visible: false } })
      .mockResolvedValueOnce({});
    const user = userEvent.setup();

    render(<App />);

    expect(await screen.findByLabelText("Advanced prompt")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hide advanced" }));
    await waitFor(() => expect(screen.queryByLabelText("Advanced prompt")).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Submit runtime" }));

    expect(mockedAPI.sendEvent).toHaveBeenLastCalledWith(
      "blocks-runtime-hidden-group",
      "submit-click",
      {
        visiblePrompt: "visible",
      },
      { hidden: ["advancedPrompt"] },
    );
  });

  it("propagates layout disabled updates to nested controls", async () => {
    mockedAPI.loadSchema.mockResolvedValue(blocksRuntimeDisabledGroupSchema());
    mockedAPI.sendEvent
      .mockResolvedValueOnce({ advanced: { __goleo_update__: true, kind: "update", disabled: true } })
      .mockResolvedValueOnce({});
    const user = userEvent.setup();

    render(<App />);

    expect(await screen.findByLabelText("Advanced prompt")).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Disable advanced" }));
    expect(await screen.findByLabelText("Advanced prompt")).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Submit runtime" }));
    expect(mockedAPI.sendEvent).toHaveBeenLastCalledWith(
      "blocks-runtime-disabled-group",
      "submit-click",
      { advancedPrompt: "secret" },
    );
  });
});

function interfaceSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "interface-1",
        kind: "interface",
        inputs: [{ id: "interface-1-input-1", type: "textbox", label: "Prompt", props: {} }],
        outputs: [{ id: "interface-1-output-1", type: "textbox", label: "Result", props: {} }],
      },
    ],
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });

  return { promise, resolve };
}

function chatSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "chat-1",
        kind: "chat",
        inputs: [{ id: "chat-1-input-1", type: "textbox", label: "Message", props: {} }],
        outputs: [{ id: "chat-1-output-1", type: "chatbot", label: "Chat", props: {} }],
      },
    ],
  };
}

function fileSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "interface-1",
        kind: "interface",
        inputs: [{ id: "interface-1-input-1", type: "file", label: "Document", props: {} }],
        outputs: [{ id: "interface-1-output-1", type: "json", label: "Result", props: {} }],
      },
    ],
  };
}

function audioSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "interface-1",
        kind: "interface",
        inputs: [
          {
            id: "interface-1-input-1",
            type: "audio",
            label: "Prompt audio",
            props: { accept: "audio/*" },
          },
        ],
        outputs: [{ id: "interface-1-output-1", type: "audio", label: "Reply audio", props: {} }],
      },
    ],
  };
}

function voiceSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "voice-1",
        kind: "voice",
        inputs: [],
        outputs: [],
      },
    ],
  };
}

function showcaseFormSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "interface-1",
        kind: "interface",
        inputs: [
          {
            id: "interface-1-input-1",
            type: "textbox",
            label: "Topic",
            props: { default: "Launch an internal support copilot for customer operations." },
          },
          {
            id: "interface-1-input-2",
            type: "number",
            label: "Max words",
            props: { default: 140, min: 80, max: 280, step: 10 },
          },
          {
            id: "interface-1-input-3",
            type: "slider",
            label: "Temperature",
            props: { default: 0.6, min: 0, max: 1, step: 0.1 },
          },
          {
            id: "interface-1-input-4",
            type: "checkbox",
            label: "Include call to action",
            props: { default: true },
          },
          {
            id: "interface-1-input-5",
            type: "dropdown",
            label: "Audience",
            props: { default: "Product leaders" },
            choices: ["Product leaders", "Support teams", "Developers"],
          },
          {
            id: "interface-1-input-6",
            type: "file",
            label: "Reference brief",
            props: {
              default: {
                id: "upload-support-brief",
                name: "support-brief.md",
                size: 1842,
                content_type: "text/markdown",
              },
            },
          },
        ],
        outputs: [
          {
            id: "interface-1-output-1",
            type: "textbox",
            label: "Launch summary",
            props: {
              default:
                "Launch summary\n\nTopic: Launch an internal support copilot for customer operations.\nAudience: Product leaders\nWord budget: 140\nTemperature: 0.6\nCTA included: true\nReference file: support-brief.md",
            },
          },
          {
            id: "interface-1-output-2",
            type: "json",
            label: "Structured output",
            props: {
              default: {
                audience: "Product leaders",
                success_metric: "Reduce repetitive ticket handling time",
              },
            },
          },
        ],
      },
    ],
  };
}

function showcaseAdapterSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "interface-1",
        kind: "interface",
        inputs: [
          {
            id: "interface-1-input-1",
            type: "textbox",
            label: "Prompt",
            props: { default: "Summarize the internal support copilot launch for team leads." },
          },
          {
            id: "interface-1-input-2",
            type: "dropdown",
            label: "Backend profile",
            props: { default: "HTTP adapter" },
            choices: ["HTTP adapter", "OpenAI-compatible", "Ollama"],
          },
          {
            id: "interface-1-input-3",
            type: "checkbox",
            label: "Streaming UX",
            props: { default: true },
          },
        ],
        outputs: [
          {
            id: "interface-1-output-1",
            type: "textbox",
            label: "Normalized result",
            props: {
              default:
                "Backend profile: HTTP adapter\nStreaming UX: true\n\nNormalized result:\nSummarize the internal support copilot launch for team leads.",
            },
          },
          {
            id: "interface-1-output-2",
            type: "json",
            label: "Backend metadata",
            props: {
              default: {
                backend: "HTTP adapter",
                transport: "normalized handler binding",
              },
            },
          },
        ],
      },
    ],
  };
}

function blocksClickSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-1",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          {
            id: "main",
            type: "group",
            label: "Generator",
            props: {},
            items: [
              { id: "name", type: "textbox", label: "Name", props: { default: "Grace" } },
              { id: "generate", type: "button", label: "Generate", props: {} },
              { id: "result", type: "textbox", label: "Result", props: {} },
            ],
          },
        ],
        events: [
          {
            id: "generate-click",
            trigger: "click",
            source: "generate",
            inputs: ["name"],
            outputs: ["result"],
          },
        ],
      },
    ],
  };
}

function blocksLoadSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-load",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [{ id: "status", type: "textbox", label: "Status", props: {} }],
        events: [{ id: "load-status", trigger: "load", inputs: [], outputs: ["status"] }],
      },
    ],
  };
}

function blocksUpdateCollisionSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-update-collision",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          { id: "check", type: "button", label: "Check", props: {} },
          { id: "result", type: "json", label: "Result", props: {} },
        ],
        events: [{ id: "check-click", trigger: "click", source: "check", inputs: [], outputs: ["result"] }],
      },
    ],
  };
}

function blocksLoadStrictSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-load-strict",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [{ id: "status", type: "textbox", label: "Status", props: {} }],
        events: [
          { id: "load-status", trigger: "load", inputs: [], outputs: ["status"] },
          { id: "ready-status", trigger: "ready", inputs: [], outputs: ["status"] },
        ],
      },
    ],
  };
}

function blocksChangeSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-change",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          { id: "prompt", type: "textbox", label: "Prompt", props: {} },
          { id: "preview", type: "textbox", label: "Preview", props: {} },
        ],
        events: [
          {
            id: "prompt-change",
            trigger: "change",
            source: "prompt",
            inputs: ["prompt"],
            outputs: ["preview"],
          },
        ],
      },
    ],
  };
}

function blocksConcurrentClickSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-concurrent-click",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          { id: "run", type: "button", label: "Run both", props: {} },
          { id: "firstResult", type: "textbox", label: "First result", props: {} },
          { id: "secondResult", type: "textbox", label: "Second result", props: {} },
        ],
        events: [
          { id: "first-click", trigger: "click", source: "run", inputs: [], outputs: ["firstResult"] },
          { id: "second-click", trigger: "click", source: "run", inputs: [], outputs: ["secondResult"] },
        ],
      },
    ],
  };
}

function blocksHiddenGroupSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-hidden-group",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          {
            id: "hidden",
            type: "group",
            label: "Hidden controls",
            props: { visible: false },
            items: [{ id: "hiddenPrompt", type: "textbox", label: "Hidden prompt", props: { default: "secret" } }],
          },
          { id: "visiblePrompt", type: "textbox", label: "Visible prompt", props: { default: "public" } },
          { id: "send", type: "button", label: "Send visible", props: {} },
        ],
        events: [
          {
            id: "send-click",
            trigger: "click",
            source: "send",
            inputs: ["hiddenPrompt", "visiblePrompt"],
            outputs: [],
          },
        ],
      },
    ],
  };
}

function blocksRuntimeHiddenGroupSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-runtime-hidden-group",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          {
            id: "advanced",
            type: "group",
            label: "Advanced controls",
            props: {},
            items: [{ id: "advancedPrompt", type: "textbox", label: "Advanced prompt", props: { default: "secret" } }],
          },
          { id: "visiblePrompt", type: "textbox", label: "Visible prompt", props: { default: "visible" } },
          { id: "hide", type: "button", label: "Hide advanced", props: {} },
          { id: "submit", type: "button", label: "Submit runtime", props: {} },
        ],
        events: [
          {
            id: "hide-advanced",
            trigger: "click",
            source: "hide",
            inputs: [],
            outputs: ["advanced"],
          },
          {
            id: "submit-click",
            trigger: "click",
            source: "submit",
            inputs: ["advancedPrompt", "visiblePrompt"],
            outputs: [],
          },
        ],
      },
    ],
  };
}

function blocksRuntimeDisabledGroupSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-runtime-disabled-group",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          {
            id: "advanced",
            type: "group",
            label: "Advanced controls",
            props: {},
            items: [{ id: "advancedPrompt", type: "textbox", label: "Advanced prompt", props: { default: "secret" } }],
          },
          { id: "disable", type: "button", label: "Disable advanced", props: {} },
          { id: "submit", type: "button", label: "Submit runtime", props: {} },
        ],
        events: [
          {
            id: "disable-advanced",
            trigger: "click",
            source: "disable",
            inputs: [],
            outputs: ["advanced"],
          },
          {
            id: "submit-click",
            trigger: "click",
            source: "submit",
            inputs: ["advancedPrompt"],
            outputs: [],
          },
        ],
      },
    ],
  };
}
