import type { ComponentSchema } from "@/types";
import { AudioPreview } from "@/features/media/AudioPreview";
import { isUploadResponse } from "./schema-utils";

export function OutputBlock({ component, value }: { component: ComponentSchema; value: unknown }) {
  if (value === undefined) {
    return null;
  }

  if ((component.type === "audio" || component.type === "image") && isUploadResponse(value)) {
    if (component.type === "image") {
      return (
        <section className="rounded-[1.25rem] border bg-background/80 p-4 shadow-sm">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold">{component.label}</h3>
            <span className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">{component.type}</span>
          </div>
          <img
            alt={value.name}
            className="max-h-72 w-full rounded-xl border bg-muted/45 object-cover"
            src={value.url}
          />
        </section>
      );
    }

    return (
      <section className="rounded-[1.25rem] border bg-background/80 p-4 shadow-sm">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">{component.label}</h3>
          <span className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">{component.type}</span>
        </div>
        <AudioPreview label={`${component.label} preview`} asset={value} />
      </section>
    );
  }

  const content =
    component.type === "json" || typeof value === "object" ? JSON.stringify(value ?? "", null, 2) : String(value ?? "");

  return (
    <section className="rounded-[1.25rem] border bg-background/80 p-4 shadow-sm">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">{component.label}</h3>
        <span className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">{component.type}</span>
      </div>
      <pre className="whitespace-pre-wrap break-words text-sm leading-6 text-foreground/90">{content}</pre>
    </section>
  );
}
