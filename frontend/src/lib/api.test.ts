import { describe, expect, it, vi } from "vitest";
import { parseServerSentEvents, predict, uploadFile } from "./api";

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
});
