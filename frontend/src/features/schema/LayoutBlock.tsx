import type { ReactNode } from "react";

export function LayoutBlock({
  type,
  label,
  children,
}: {
  type: string;
  label: string;
  children: ReactNode;
}) {
  if (type === "row") {
    return <div className="grid gap-5 rounded-[1.25rem] border bg-background/70 p-4 sm:grid-cols-2">{children}</div>;
  }

  if (type === "column") {
    return <div className="grid gap-5">{children}</div>;
  }

  return (
    <section className="grid gap-4 rounded-[1.25rem] border bg-background/70 p-4">
      <div className="text-xs uppercase tracking-[0.16em] text-muted-foreground">{label}</div>
      <div className="grid gap-4">{children}</div>
    </section>
  );
}
