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
