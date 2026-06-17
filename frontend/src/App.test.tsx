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
    mockedAPI.stream.mockImplementation(async (_id, _data, onChunk) => {
      onChunk("Hel");
      onChunk("lo");
    });
    const user = userEvent.setup();

    render(<App />);

    await user.type(await screen.findByLabelText("Message"), "Hi{Enter}");

    expect(mockedAPI.stream).toHaveBeenCalledWith("chat-1", ["Hi"], expect.any(Function));
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
