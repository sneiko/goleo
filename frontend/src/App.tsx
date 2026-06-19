import { useEffect, useState } from "react";
import { Empty, EmptyDescription, EmptyTitle } from "@/components/ui/empty";
import { ErrorAlert } from "@/components/ErrorAlert";
import { LoadingState } from "@/components/LoadingState";
import { Metric } from "@/components/Metric";
import { loadSchema } from "@/lib/api";
import type { AppSchema } from "@/types";
import { BlocksInterface } from "@/features/interfaces/BlocksInterface";
import { ChatInterface } from "@/features/interfaces/ChatInterface";
import { FormInterface } from "@/features/interfaces/FormInterface";
import { VoiceInterface } from "@/features/interfaces/VoiceInterface";
import { readDemoMode } from "@/features/demo/demo-mode";
import { errorMessage } from "@/features/schema/schema-utils";

export default function App() {
  const [schema, setSchema] = useState<AppSchema | null>(null);
  const [error, setError] = useState<string | null>(null);
  const demoMode = readDemoMode();

  useEffect(() => {
    let active = true;
    loadSchema()
      .then((nextSchema) => {
        if (active) {
          setSchema(nextSchema);
        }
      })
      .catch((loadError: unknown) => {
        if (active) {
          setError(errorMessage(loadError));
        }
      });

    return () => {
      active = false;
    };
  }, []);

  return (
    <main className="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
        <header className="rounded-[1.25rem] border bg-card/85 p-6 shadow-sm backdrop-blur-sm">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="max-w-3xl">
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-accent">Goleo showcase</p>
              <h1 className="mt-2 text-4xl font-semibold tracking-normal sm:text-5xl">AI demos from Go functions</h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
                Wrap Go handlers, streaming chat flows, and adapter-backed tools in a single embedded web app.
              </p>
            </div>
            <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
              <Metric label="Surface" value="Embedded UI" />
              <Metric label="Interfaces" value={String(schema?.interfaces.length ?? 0)} />
              <Metric label="Runtime" value="Go-first" />
            </dl>
          </div>
        </header>

        {error ? <ErrorAlert title="Could not load app schema" message={error} /> : null}
        {!schema && !error ? <LoadingState /> : null}
        {schema && schema.interfaces.length === 0 ? (
          <Empty>
            <EmptyTitle>No interfaces registered</EmptyTitle>
            <EmptyDescription>Add an Interface or Chat in Go to render controls here.</EmptyDescription>
          </Empty>
        ) : null}
        {schema?.interfaces.map((iface) =>
          iface.kind === "chat" ? (
            <ChatInterface key={iface.id} iface={iface} demoMode={demoMode} />
          ) : iface.kind === "voice" ? (
            <VoiceInterface key={iface.id} iface={iface} />
          ) : iface.kind === "blocks" ? (
            <BlocksInterface key={iface.id} iface={iface} />
          ) : (
            <FormInterface key={iface.id} iface={iface} demoMode={demoMode} />
          ),
        )}
      </div>
    </main>
  );
}
