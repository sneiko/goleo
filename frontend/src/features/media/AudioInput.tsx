import { useState } from "react";
import { Mic, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { uploadFile } from "@/lib/api";
import type { ComponentSchema, UploadResponse } from "@/types";
import { errorMessage, extensionFromMime, isUploadResponse, stringProp } from "@/features/schema/schema-utils";
import { AudioPreview } from "./AudioPreview";
import { useAudioRecorder } from "./audio-hooks";

export function AudioInput({
  component,
  disabled,
  value,
  onChange,
}: {
  component: ComponentSchema;
  disabled: boolean;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const props = component.props ?? {};
  const [upload, setUpload] = useState<UploadResponse | null>(() => (isUploadResponse(value) ? value : null));
  const [error, setError] = useState<string | null>(null);
  const [isUploading, setIsUploading] = useState(false);

  const recorder = useAudioRecorder({
    onBlob: async (blob, mimeType) => {
      const file = new File([blob], `recording${extensionFromMime(mimeType)}`, { type: mimeType });
      await persistUpload(file);
    },
    onError: (message) => setError(message),
  });

  async function persistUpload(file: File) {
    setError(null);
    setIsUploading(true);
    try {
      const result = await uploadFile(file);
      setUpload(result);
      onChange(result);
    } catch (uploadError) {
      setError(errorMessage(uploadError));
    } finally {
      setIsUploading(false);
    }
  }

  async function onFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }

    await persistUpload(file);
  }

  return (
    <Field data-disabled={disabled || undefined}>
      <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
      <div className="flex flex-wrap gap-3">
        <Input
          id={component.id}
          aria-label={component.label}
          accept={stringProp(props.accept) ?? "audio/*"}
          disabled={disabled || isUploading || props.disabled === true}
          type="file"
          onChange={(event) => void onFileChange(event)}
        />
        <Button type="button" variant="secondary" disabled={disabled || isUploading || recorder.isRecording} onClick={() => void recorder.start()}>
          <Mic data-icon="inline-start" />
          Record
        </Button>
        <Button type="button" variant="outline" disabled={!recorder.isRecording} onClick={() => recorder.stop()}>
          <Square data-icon="inline-start" />
          Stop recording
        </Button>
      </div>
      {isUploading ? <FieldDescription>Uploading...</FieldDescription> : null}
      {upload ? <AudioPreview label={`${component.label} preview`} asset={upload} /> : null}
      {error ? <FieldDescription className="text-destructive">{error}</FieldDescription> : null}
    </Field>
  );
}
