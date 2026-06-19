import { useState } from "react";
import { FileText } from "lucide-react";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { uploadFile } from "@/lib/api";
import type { ComponentSchema, UploadResponse } from "@/types";
import { errorMessage, isUploadResponse, stringProp } from "@/features/schema/schema-utils";

export function FileInput({
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

  async function onFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }

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

  return (
    <Field data-disabled={disabled || undefined}>
      <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
      <Input
        id={component.id}
        aria-label={component.label}
        accept={stringProp(props.accept)}
        disabled={disabled || isUploading || props.disabled === true}
        multiple={Boolean(props.multiple)}
        type="file"
        onChange={(event) => void onFileChange(event)}
      />
      {isUploading ? <FieldDescription>Uploading...</FieldDescription> : null}
      {upload ? (
        <div className="flex items-center gap-2 rounded-2xl border bg-muted/40 px-3 py-2 text-sm shadow-sm">
          <FileText data-icon="inline-start" />
          <span className="font-medium">{upload.name}</span>
          <span className="text-muted-foreground">
            {upload.size} B {upload.content_type}
          </span>
        </div>
      ) : null}
      {error ? <FieldDescription className="text-destructive">{error}</FieldDescription> : null}
    </Field>
  );
}
