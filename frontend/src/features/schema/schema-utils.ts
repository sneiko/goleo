import { useMemo, useState } from "react";
import type { ComponentSchema, UploadResponse } from "@/types";

export type Values = Record<string, unknown>;
export type Outputs = Record<string, unknown>;

export function isLayoutComponent(type: string) {
  return type === "row" || type === "column" || type === "group";
}

export function flattenLeafComponents(components: ComponentSchema[]): ComponentSchema[] {
  const result: ComponentSchema[] = [];
  for (const component of components) {
    if (isLayoutComponent(component.type)) {
      result.push(...flattenLeafComponents(component.items ?? []));
      continue;
    }
    result.push(component);
  }

  return result;
}

export function useInitialValues(components: ComponentSchema[]) {
  const initialValues = useMemo(
    () =>
      Object.fromEntries(
        components.map((component) => {
          const props = component.props ?? {};
          if (component.type === "checkbox") {
            return [component.id, Boolean(props.default ?? false)];
          }
          return [component.id, props.default ?? ""];
        }),
      ),
    [components],
  );

  return useState<Values>(initialValues);
}

export function isUploadResponse(value: unknown): value is UploadResponse {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const upload = value as Partial<UploadResponse>;
  return (
    typeof upload.id === "string" &&
    typeof upload.name === "string" &&
    typeof upload.size === "number" &&
    typeof upload.content_type === "string"
  );
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function stringProp(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

export function numberProp(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}

export function normalizeStreamValue(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  if (value === null || value === undefined) {
    return "";
  }

  return JSON.stringify(value);
}

export function extensionFromMime(mimeType: string) {
  switch (mimeType) {
    case "audio/wav":
      return ".wav";
    case "audio/mp3":
    case "audio/mpeg":
      return ".mp3";
    default:
      return ".webm";
  }
}

export async function blobToBase64(blob: Blob) {
  const buffer =
    typeof blob.arrayBuffer === "function" ? await blob.arrayBuffer() : await new Response(blob).arrayBuffer();
  let binary = "";
  const bytes = new Uint8Array(buffer);
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

export function supportsAudioRecording() {
  return typeof window !== "undefined" && typeof MediaRecorder !== "undefined" && Boolean(navigator.mediaDevices?.getUserMedia);
}
