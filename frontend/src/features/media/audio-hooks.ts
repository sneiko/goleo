import { useEffect, useRef, useState } from "react";
import { errorMessage, supportsAudioRecording } from "@/features/schema/schema-utils";

export function useAudioRecorder({
  onBlob,
  onError,
}: {
  onBlob: (blob: Blob, mimeType: string) => Promise<void>;
  onError: (message: string) => void;
}) {
  const recorderRef = useRef<MediaRecorder | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const [isRecording, setIsRecording] = useState(false);

  useEffect(() => {
    return () => {
      streamRef.current?.getTracks().forEach((track) => track.stop());
    };
  }, []);

  async function start() {
    if (!supportsAudioRecording()) {
      onError("Audio recording is not supported in this browser.");
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      chunksRef.current = [];
      const recorder = new MediaRecorder(stream);
      recorderRef.current = recorder;
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          chunksRef.current.push(event.data);
        }
      };
      recorder.onstop = () => {
        const mimeType = recorder.mimeType || "audio/webm";
        const blob = new Blob(chunksRef.current, { type: mimeType });
        setIsRecording(false);
        stream.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
        void onBlob(blob, mimeType).catch((error) => onError(errorMessage(error)));
      };
      recorder.start();
      setIsRecording(true);
    } catch (error) {
      onError(errorMessage(error));
    }
  }

  function stop() {
    recorderRef.current?.stop();
  }

  return { isRecording, start, stop };
}

export function useStreamingAudioRecorder({
  onChunk,
  onStop,
  onError,
}: {
  onChunk: (blob: Blob, mimeType: string) => Promise<void>;
  onStop: () => Promise<void>;
  onError: (message: string) => void;
}) {
  const recorderRef = useRef<MediaRecorder | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const [isRecording, setIsRecording] = useState(false);

  useEffect(() => {
    return () => {
      streamRef.current?.getTracks().forEach((track) => track.stop());
    };
  }, []);

  async function start() {
    if (!supportsAudioRecording()) {
      onError("Audio recording is not supported in this browser.");
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      const recorder = new MediaRecorder(stream);
      recorderRef.current = recorder;
      recorder.ondataavailable = (event) => {
        if (event.data.size === 0) {
          return;
        }

        const mimeType = recorder.mimeType || "audio/webm";
        void onChunk(event.data, mimeType).catch((error) => onError(errorMessage(error)));
      };
      recorder.onstop = () => {
        setIsRecording(false);
        stream.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
        recorderRef.current = null;
        void onStop().catch((error) => onError(errorMessage(error)));
      };
      recorder.start(250);
      setIsRecording(true);
    } catch (error) {
      onError(errorMessage(error));
    }
  }

  function stop() {
    recorderRef.current?.stop();
  }

  return { isRecording, start, stop };
}
