import { useEffect, useRef, useState } from "react";
import { Mic, PhoneCall, Square } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { openVoiceSession } from "@/lib/api";
import type { InterfaceSchema, UploadResponse, VoiceServerEvent, VoiceSessionConnection } from "@/types";
import { AudioPreview } from "@/features/media/AudioPreview";
import { useStreamingAudioRecorder } from "@/features/media/audio-hooks";
import { blobToBase64 } from "@/features/schema/schema-utils";

type VoiceTranscriptItem = {
  kind: "system" | "assistant";
  content: string;
};

export function VoiceInterface({ iface }: { iface: InterfaceSchema }) {
  const [connectionState, setConnectionState] = useState<"disconnected" | "connecting" | "connected">("disconnected");
  const [transcript, setTranscript] = useState<VoiceTranscriptItem[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [latestAudio, setLatestAudio] = useState<UploadResponse | null>(null);
  const [speakerState, setSpeakerState] = useState("idle");
  const connectionRef = useRef<VoiceSessionConnection | null>(null);
  const sequenceRef = useRef(0);

  const recorder = useStreamingAudioRecorder({
    onChunk: async (blob, mimeType) => {
      const connection = connectionRef.current;
      if (!connection) {
        throw new Error("voice session is not connected");
      }

      const data = await blobToBase64(blob);
      sequenceRef.current += 1;
      connection.send({
        type: "input.audio",
        audio: {
          mime_type: mimeType,
          sequence: sequenceRef.current,
          data,
        },
      });
    },
    onStop: async () => {
      connectionRef.current?.send({ type: "input.stop" });
    },
    onError: (message) => setError(message),
  });

  useEffect(() => {
    return () => {
      connectionRef.current?.close();
      connectionRef.current = null;
    };
  }, []);

  function onVoiceEvent(event: VoiceServerEvent) {
    switch (event.type) {
      case "session.ready":
        setConnectionState("connected");
        setSpeakerState("idle");
        setTranscript((current) => [...current, { kind: "system", content: "Voice session ready." }]);
        return;
      case "session.closed":
        setConnectionState("disconnected");
        setTranscript((current) => [...current, { kind: "system", content: "Voice session closed." }]);
        connectionRef.current?.close();
        connectionRef.current = null;
        return;
      case "output.text":
        if (event.text) {
          const text = event.text;
          setTranscript((current) => [...current, { kind: "assistant", content: text }]);
        }
        return;
      case "output.audio":
        if (event.audio) {
          setLatestAudio(event.audio);
          setSpeakerState("ready");
        }
        return;
      case "output.state":
        if (event.state) {
          setSpeakerState(event.state);
          setTranscript((current) => [...current, { kind: "system", content: `Output state: ${event.state}` }]);
        }
        return;
      case "error":
        setError(event.text ?? "voice session failed");
        return;
      default:
        return;
    }
  }

  function connect() {
    if (connectionRef.current) {
      return;
    }

    setError(null);
    setConnectionState("connecting");

    const connection = openVoiceSession(iface.id, {
      onEvent: onVoiceEvent,
      onError: (nextError) => {
        setError(nextError.message);
        setConnectionState("disconnected");
        connectionRef.current = null;
      },
      onClose: () => {
        setConnectionState("disconnected");
        connectionRef.current = null;
      },
    });

    connectionRef.current = connection;
    connection.send({ type: "session.start" });
  }

  function disconnect() {
    connectionRef.current?.send({ type: "session.close" });
  }

  function interrupt() {
    connectionRef.current?.send({ type: "output.interrupt" });
  }

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Voice</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6 pt-6 lg:grid-cols-[0.9fr,1.1fr]">
        <section className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="rounded-[1.25rem] border bg-muted/35 p-4">
              <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Connection</p>
              <p className="mt-2 text-sm font-medium capitalize">{connectionState}</p>
            </div>
            <div className="rounded-[1.25rem] border bg-muted/35 p-4">
              <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Microphone</p>
              <p className="mt-2 text-sm font-medium">{recorder.isRecording ? "Live" : "Muted"}</p>
            </div>
            <div className="rounded-[1.25rem] border bg-muted/35 p-4">
              <p className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Speaker</p>
              <p className="mt-2 text-sm font-medium capitalize">{speakerState}</p>
            </div>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button type="button" disabled={connectionState !== "disconnected"} onClick={connect}>
              <PhoneCall data-icon="inline-start" />
              Connect voice
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={connectionState !== "connected" || recorder.isRecording}
              onClick={() => void recorder.start()}
            >
              <Mic data-icon="inline-start" />
              Unmute mic
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={!recorder.isRecording}
              onClick={() => recorder.stop()}
            >
              <Square data-icon="inline-start" />
              Mute mic
            </Button>
            <Button type="button" variant="outline" disabled={connectionState === "disconnected"} onClick={interrupt}>
              Interrupt
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={connectionState !== "connected" || recorder.isRecording}
              onClick={disconnect}
            >
              Disconnect
            </Button>
          </div>
          {error ? <ErrorAlert title="Voice session failed" message={error} /> : null}
          {latestAudio ? <AudioPreview label="Voice reply preview" asset={latestAudio} /> : null}
        </section>
        <section
          aria-label="Voice transcript"
          role="log"
          className="flex min-h-72 flex-col gap-3 rounded-[1.25rem] border bg-muted/35 p-4"
        >
          {transcript.length === 0 ? (
            <p className="text-sm leading-6 text-muted-foreground">Connect the voice session to start exchanging events.</p>
          ) : (
            transcript.map((item, index) => (
              <div
                key={`${item.kind}-${index}`}
                className={
                  item.kind === "assistant"
                    ? "mr-auto max-w-[85%] rounded-3xl rounded-bl-md border bg-card px-4 py-3 text-sm leading-6 shadow-sm"
                    : "max-w-[85%] rounded-3xl border bg-background px-4 py-3 text-sm leading-6 text-muted-foreground"
                }
              >
                {item.content}
              </div>
            ))
          )}
        </section>
      </CardContent>
    </Card>
  );
}
