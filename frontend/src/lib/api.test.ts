import { describe, expect, it, vi } from "vitest";
import { parseServerSentEvents, predict, sendEvent, uploadFile } from "./api";

describe("api client", () => {
  it("posts predict requests using the existing wire format", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url, _init) => {
        return new Response(JSON.stringify({ data: ["Hello Ada"] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    const result = await predict("interface-1", ["Ada"]);

    expect(result).toEqual(["Hello Ada"]);
    expect(fetch).toHaveBeenCalledWith("/api/predict", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ interface_id: "interface-1", data: ["Ada"] }),
    });
  });

  it("extracts message data from server-sent event chunks", () => {
    expect(
      parseServerSentEvents(
        'data: Hel\n\n\n\ndata: {"text":"ignored object"}\n\ndata: lo\\nthere\n\n',
      ),
    ).toEqual(["Hel", '{"text":"ignored object"}', "lo\nthere"]);
  });

  it("uploads files using the existing multipart field name", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url, _init) => {
        return new Response(
          JSON.stringify({
            id: "upload-prompt.txt-5",
            name: "prompt.txt",
            size: 5,
            content_type: "text/plain",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }),
    );

    const file = new File(["hello"], "prompt.txt", { type: "text/plain" });
    const result = await uploadFile(file);

    expect(result.name).toBe("prompt.txt");
    expect(fetch).toHaveBeenCalledWith(
      "/api/upload",
      expect.objectContaining({
        method: "POST",
        body: expect.any(FormData),
      }),
    );
    const body = vi.mocked(fetch).mock.calls[0][1]?.body as FormData;
    expect(body.get("file")).toBe(file);
  });

  it("posts event requests using the Blocks wire format and returns component data", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url, _init) => {
        return new Response(
          JSON.stringify({
            data: {
              "blocks-1-component-3": "Done",
              "blocks-1-component-4": { kind: "update", value: "ready", disabled: false },
            },
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }),
    );

    const result = await sendEvent("blocks-1", "blocks-1-event-1", { "blocks-1-component-1": "Ada" });

    expect(result).toEqual({
      "blocks-1-component-3": "Done",
      "blocks-1-component-4": { kind: "update", value: "ready", disabled: false },
    });
    expect(fetch).toHaveBeenCalledWith("/api/event", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        interface_id: "blocks-1",
        event_id: "blocks-1-event-1",
        data: { "blocks-1-component-1": "Ada" },
      }),
    });
  });

  it("includes request_id when sending event requests with a request id", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url, _init) => {
        return new Response(JSON.stringify({ data: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    await sendEvent("interface-1", "submit", {}, { requestID: "request-1" });

    expect(fetch).toHaveBeenCalledWith("/api/event", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        interface_id: "interface-1",
        event_id: "submit",
        data: {},
        request_id: "request-1",
      }),
    });
  });

  it("includes hidden ids only when sending event requests with hidden inputs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url, _init) => {
        return new Response(JSON.stringify({ data: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    await sendEvent(
      "blocks-1",
      "blocks-1-event-1",
      { "blocks-1-component-2": "visible" },
      { hidden: ["blocks-1-component-1"] },
    );

    expect(fetch).toHaveBeenCalledWith("/api/event", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        interface_id: "blocks-1",
        event_id: "blocks-1-event-1",
        data: { "blocks-1-component-2": "visible" },
        hidden: ["blocks-1-component-1"],
      }),
    });
  });

  it("throws backend error messages for event request failures", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url, _init) => {
        return new Response(JSON.stringify({ error: { message: "event failed" } }), {
          status: 400,
          statusText: "Bad Request",
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    await expect(sendEvent("interface-1", "submit", {})).rejects.toThrow("event failed");
  });
});
