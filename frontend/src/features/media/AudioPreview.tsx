import { FileAudio } from "lucide-react";
import type { UploadResponse } from "@/types";

export function AudioPreview({ label, asset }: { label: string; asset: UploadResponse }) {
  return (
    <div className="flex flex-col gap-3 rounded-2xl border bg-muted/40 p-3 shadow-sm">
      <div className="flex items-center gap-2 text-sm">
        <FileAudio data-icon="inline-start" />
        <span className="font-medium">{asset.name}</span>
        <span className="text-muted-foreground">
          {asset.size} B {asset.content_type}
        </span>
      </div>
      {asset.url ? <audio aria-label={label} controls src={asset.url} /> : null}
    </div>
  );
}
