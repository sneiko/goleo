import { type FormEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, FileAudio, FileText, Mic, PhoneCall, Send, Square } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Empty, EmptyDescription, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Slider } from "@/components/ui/slider";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import {
  loadSchema,
  openVoiceSession,
  cancelRequest,
  sendEvent,
  streamWithEvents,
  uploadFile,
  predict,
} from "@/lib/api";
import type {
  AppSchema,
  ComponentUpdate,
  ComponentSchema,
  EventSchema,
  InterfaceSchema,
  StreamEvent,
  UploadResponse,
  VoiceServerEvent,
  VoiceSessionConnection,
} from "@/types";

type DemoMode =
  | "none"
  | "readme-hero"
  | "readme-components"
  | "readme-outputs"
  | "readme-chat"
  | "readme-adapters"
  | "readme-mobile";

type Values = Record<string, unknown>;
type Outputs = Record<string, unknown>;
type ChatMessage = {
  role: "user" | "assistant";
  content: string;
};

type VoiceTranscriptItem = {
  kind: "system" | "assistant";
  content: string;
};

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

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border bg-background/75 px-3 py-2 shadow-sm">
      <dt className="text-xs uppercase tracking-[0.14em] text-muted-foreground">{label}</dt>
      <dd className="mt-1 font-medium">{value}</dd>
    </div>
  );
}

function LoadingState() {
  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64" />
      </CardHeader>
      <CardContent className="grid gap-4 pt-6 lg:grid-cols-[1.1fr,0.9fr]">
        <div className="flex flex-col gap-3">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-9 w-28" />
        </div>
        <div className="flex flex-col gap-3">
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      </CardContent>
    </Card>
  );
}

function FormInterface({ iface, demoMode }: { iface: InterfaceSchema; demoMode: DemoMode }) {
  const isOutputShowcase = demoMode === "readme-outputs";
  const renderedInputs = isOutputShowcase ? outputShowcaseInputs(iface.inputs) : iface.inputs;
  const inputFields = flattenLeafComponents(renderedInputs);
  const outputFields = flattenLeafComponents(iface.outputs);
  const [values, setValues] = useInitialValues(inputFields);
  const [outputs, setOutputs] = useState<Outputs>(() => initialOutputs(outputFields, demoMode));
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const hasOutputValues = outputFields.some((component) => outputs[component.id] !== undefined);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIsSubmitting(true);
    try {
      const result = await predict(
        iface.id,
        inputFields.map((component) => values[component.id] ?? ""),
      );
      setOutputs(Object.fromEntries(outputFields.map((component, index) => [component.id, result[index]])));
    } catch (submitError) {
      setError(errorMessage(submitError));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Interface</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <form onSubmit={onSubmit}>
        <CardContent
          className={
            hasOutputValues
              ? isOutputShowcase
                ? "grid gap-6 pt-6 lg:grid-cols-[0.78fr,1.22fr]"
                : "grid gap-6 pt-6 lg:grid-cols-[1.08fr,0.92fr]"
              : "pt-6"
          }
        >
          <FieldGroup className="gap-5">
            {renderSchemaInputs(renderedInputs, isSubmitting, values, (component, value) => {
              setValues((current) => ({ ...current, [component.id]: value }));
            })}
            {error ? <ErrorAlert title="Request failed" message={error} /> : null}
          </FieldGroup>
          {hasOutputValues ? (
            <section className="flex flex-col gap-3">
              {renderSchemaOutputs(iface.outputs, outputs).map((node) => (
                <OutputBlock key={node.key} component={node.component} value={node.value} />
              ))}
            </section>
          ) : null}
        </CardContent>
        <CardFooter className="border-t bg-card/55">
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? <Spinner /> : null}
            {isSubmitting ? "Running" : "Run"}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

type ComponentRuntimeState = {
  visible?: boolean;
  disabled?: boolean;
  choices?: string[];
  label?: string;
};

function BlocksInterface({ iface }: { iface: InterfaceSchema }) {
  const components = iface.components ?? [];
  const events = iface.events ?? [];
  const leafComponents = useMemo(() => flattenLeafComponents(components), [components]);
  const componentByID = useMemo(() => new Map(leafComponents.map((component) => [component.id, component])), [leafComponents]);
  const eventInputIDs = useMemo(() => new Set(events.flatMap((event) => event.inputs)), [events]);
  const eventOutputIDs = useMemo(() => new Set(events.flatMap((event) => event.outputs)), [events]);
  const loadEvents = useMemo(
    () => events.filter((event) => event.trigger === "load"),
    [events],
  );
  const [values, setValues] = useInitialValues(leafComponents);
  const [runtime, setRuntime] = useState<Record<string, ComponentRuntimeState>>({});
  const [error, setError] = useState<string | null>(null);
  const [runningSources, setRunningSources] = useState<Record<string, number>>({});
  const sentLoadEventsRef = useRef(false);
  const hiddenComponentIDs = useMemo(() => hiddenBlockComponentIDs(components, runtime), [components, runtime]);

  function applyEventResponse(response: Record<string, unknown>) {
    const nextValues: Values = {};
    const nextRuntime: Record<string, ComponentRuntimeState> = {};

    for (const [componentID, result] of Object.entries(response)) {
      if (isComponentUpdate(result)) {
        if (result.value !== undefined) {
          nextValues[componentID] = result.value;
        }
        const runtimeUpdate = componentRuntimeUpdate(result);
        if (runtimeUpdate) {
          nextRuntime[componentID] = runtimeUpdate;
        }
        continue;
      }

      nextValues[componentID] = result;
    }

    if (Object.keys(nextValues).length > 0) {
      setValues((current) => ({ ...current, ...nextValues }));
    }

    if (Object.keys(nextRuntime).length > 0) {
      setRuntime((current) => {
        const merged = { ...current };
        for (const [componentID, update] of Object.entries(nextRuntime)) {
          merged[componentID] = { ...merged[componentID], ...update };
        }
        return merged;
      });
    }
  }

  async function runBlockEvent(event: EventSchema, sourceID: string | undefined, nextValues: Values = values) {
    setError(null);
    if (sourceID) {
      setRunningSources((current) => ({
        ...current,
        [sourceID]: (current[sourceID] ?? 0) + 1,
      }));
    }

    try {
      const hiddenInputs = hiddenEventInputIDs(event, hiddenComponentIDs);
      const requestData = blockEventPayload(event, nextValues, componentByID, hiddenComponentIDs);
      const response =
        hiddenInputs.length > 0
          ? await sendEvent(iface.id, event.id, requestData, { hidden: hiddenInputs })
          : await sendEvent(iface.id, event.id, requestData);
      applyEventResponse(response);
    } catch (eventError) {
      setError(errorMessage(eventError));
    } finally {
      if (sourceID) {
        setRunningSources((current) => {
          const next = { ...current };
          const count = (next[sourceID] ?? 0) - 1;
          if (count > 0) {
            next[sourceID] = count;
          } else {
            delete next[sourceID];
          }
          return next;
        });
      }
    }
  }

  useEffect(() => {
    if (sentLoadEventsRef.current) {
      return;
    }
    sentLoadEventsRef.current = true;
    for (const event of loadEvents) {
      void runBlockEvent(event, event.source);
    }
  }, [loadEvents]);

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Blocks</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-6">
        {renderBlocksComponents(components, {
          eventInputIDs,
          eventOutputIDs,
          onButtonClick: (component) => {
            for (const event of events.filter((candidate) => candidate.trigger === "click" && candidate.source === component.id)) {
              void runBlockEvent(event, component.id);
            }
          },
          onValueChange: (component, value) => {
            const nextValues = { ...values, [component.id]: value };
            setValues(nextValues);
            for (const event of events.filter((candidate) => candidate.trigger === "change" && candidate.source === component.id)) {
              void runBlockEvent(event, component.id, nextValues);
            }
          },
          runningSources,
          runtime,
          values,
          inheritedDisabled: false,
        })}
        {error ? <ErrorAlert title="Event failed" message={error} /> : null}
      </CardContent>
    </Card>
  );
}

function renderBlocksComponents(
  components: ComponentSchema[],
  context: {
    eventInputIDs: Set<string>;
    eventOutputIDs: Set<string>;
    onButtonClick: (component: ComponentSchema) => void;
    onValueChange: (component: ComponentSchema, value: unknown) => void;
    runningSources: Record<string, number>;
    runtime: Record<string, ComponentRuntimeState>;
    values: Values;
    inheritedDisabled: boolean;
  },
): ReactNode[] {
  return components.map((component) => {
    const effectiveComponent = applyComponentRuntime(component, context.runtime[component.id]);
    const props = effectiveComponent.props ?? {};
    if (props.visible === false || effectiveComponent.type === "state") {
      return null;
    }

    if (isLayoutComponent(effectiveComponent.type)) {
      const disabled = context.inheritedDisabled || props.disabled === true;
      return (
        <LayoutBlock key={effectiveComponent.id} type={effectiveComponent.type} label={effectiveComponent.label}>
          {renderBlocksComponents(effectiveComponent.items ?? [], { ...context, inheritedDisabled: disabled })}
        </LayoutBlock>
      );
    }

    const isRunning = (context.runningSources[effectiveComponent.id] ?? 0) > 0;
    const disabled = context.inheritedDisabled || isRunning || props.disabled === true;

    if (effectiveComponent.type === "button") {
      return (
        <Button
          key={effectiveComponent.id}
          type="button"
          disabled={disabled}
          onClick={() => context.onButtonClick(effectiveComponent)}
        >
          {isRunning ? <Spinner /> : null}
          {effectiveComponent.label}
        </Button>
      );
    }

    const isOutputOnly =
      context.eventOutputIDs.has(effectiveComponent.id) && !context.eventInputIDs.has(effectiveComponent.id);
    if (isOutputOnly || !isEditableBlocksComponent(effectiveComponent.type)) {
      return (
        <OutputBlock key={effectiveComponent.id} component={effectiveComponent} value={context.values[effectiveComponent.id]} />
      );
    }

    return (
      <SchemaInput
        key={effectiveComponent.id}
        component={effectiveComponent}
        disabled={disabled}
        value={context.values[effectiveComponent.id]}
        onChange={(value) => context.onValueChange(effectiveComponent, value)}
      />
    );
  });
}

function applyComponentRuntime(component: ComponentSchema, runtime?: ComponentRuntimeState): ComponentSchema {
  if (!runtime) {
    return component;
  }

  const props = { ...(component.props ?? {}) };
  if (runtime.visible !== undefined) {
    props.visible = runtime.visible;
  }
  if (runtime.disabled !== undefined) {
    props.disabled = runtime.disabled;
  }

  return {
    ...component,
    choices: runtime.choices ?? component.choices,
    label: runtime.label ?? component.label,
    props,
  };
}

function blockEventPayload(
  event: EventSchema,
  values: Values,
  componentByID: Map<string, ComponentSchema>,
  hiddenComponentIDs: Set<string>,
) {
  const payload: Record<string, unknown> = {};

  for (const componentID of event.inputs) {
    const component = componentByID.get(componentID);
    if (!component) {
      continue;
    }

    if (component.type === "state" || hiddenComponentIDs.has(componentID)) {
      continue;
    }

    const value = values[componentID];
    if (value !== undefined) {
      payload[componentID] = value;
    }
  }

  return payload;
}

function hiddenEventInputIDs(event: EventSchema, hiddenComponentIDs: Set<string>) {
  return event.inputs.filter((componentID) => hiddenComponentIDs.has(componentID));
}

function hiddenBlockComponentIDs(
  components: ComponentSchema[],
  runtime: Record<string, ComponentRuntimeState>,
  ancestorHidden = false,
) {
  const hiddenIDs = new Set<string>();

  for (const component of components) {
    const effectiveComponent = applyComponentRuntime(component, runtime[component.id]);
    const hidden = ancestorHidden || effectiveComponent.props?.visible === false;
    if (hidden) {
      hiddenIDs.add(component.id);
    }

    for (const hiddenID of hiddenBlockComponentIDs(effectiveComponent.items ?? [], runtime, hidden)) {
      hiddenIDs.add(hiddenID);
    }
  }

  return hiddenIDs;
}

function componentRuntimeUpdate(update: ComponentUpdate): ComponentRuntimeState | null {
  const runtimeUpdate: ComponentRuntimeState = {};
  if (update.visible !== undefined) {
    runtimeUpdate.visible = update.visible;
  }
  if (update.disabled !== undefined) {
    runtimeUpdate.disabled = update.disabled;
  }
  if (update.choices !== undefined) {
    runtimeUpdate.choices = update.choices;
  }
  if (update.label !== undefined) {
    runtimeUpdate.label = update.label;
  }

  return Object.keys(runtimeUpdate).length > 0 ? runtimeUpdate : null;
}

function isComponentUpdate(value: unknown): value is ComponentUpdate {
  return Boolean(
    value &&
      typeof value === "object" &&
      (value as { __goleo_update__?: unknown }).__goleo_update__ === true &&
      (value as { kind?: unknown }).kind === "update",
  );
}

function isEditableBlocksComponent(type: string) {
  return (
    type === "textbox" ||
    type === "number" ||
    type === "slider" ||
    type === "checkbox" ||
    type === "dropdown" ||
    type === "audio" ||
    type === "file" ||
    type === "image"
  );
}

function ChatInterface({ iface, demoMode }: { iface: InterfaceSchema; demoMode: DemoMode }) {
  const [message, setMessage] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>(() => initialChatMessages(demoMode));
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [streamPhase, setStreamPhase] = useState<"idle" | "queued" | "running">("idle");
  const [streamRequestID, setStreamRequestID] = useState<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const baseMessages = useRef<ChatMessage[]>(initialChatMessages(demoMode));
  const input = iface.inputs[0] ?? { id: `${iface.id}-message`, type: "textbox", label: "Message", props: {} };

  async function submitMessage() {
    const trimmed = message.trim();
    if (trimmed === "" || isSubmitting) {
      return;
    }

    setError(null);
    setMessage("");
    setIsSubmitting(true);
    setStreamPhase("queued");
    setMessages((current) => [...current, { role: "user", content: trimmed }, { role: "assistant", content: "" }]);

    const controller = new AbortController();
    abortControllerRef.current = controller;
    setStreamRequestID(null);

    try {
      await streamWithEvents(
        iface.id,
        [trimmed],
        (event) => {
          if (event.status === "running") {
            setStreamPhase("running");
          }

          if (event.status === "queued") {
            setStreamPhase("queued");
          }

          if (event.status === "error") {
            setStreamPhase("idle");
            if (event.error) {
              setError(event.error);
            }
            return;
          }

          if (event.data === undefined) {
            return;
          }

          const chunk = normalizeStreamValue(event.data);
          if (chunk === "") {
            return;
          }

          setMessages((current) => {
            const next = [...current];
            const last = next[next.length - 1];
            if (last?.role === "assistant") {
              next[next.length - 1] = { ...last, content: last.content + chunk };
            }
            return next;
          });
        },
        {
          signal: controller.signal,
          onRequestID: setStreamRequestID,
        },
      );
    } catch (streamError) {
      if (streamError instanceof DOMException && streamError.name === "AbortError") {
        return;
      }

      setError(errorMessage(streamError));
      setStreamPhase("idle");
    } finally {
      setIsSubmitting(false);
      setStreamRequestID(null);
      setStreamPhase("idle");
    }
  }

  async function stopGeneration() {
    abortControllerRef.current?.abort();
    if (streamRequestID) {
      await cancelRequest(streamRequestID).catch((error) => {
        if (error instanceof Error) {
          setError(error.message);
        }
      });
      setStreamRequestID(null);
    }
  }

  function clearHistory() {
    setMessages(baseMessages.current);
    setError(null);
  }

  function copyHistory() {
    void navigator.clipboard.writeText(
      messages
        .map((entry) => `${entry.role === "user" ? "You" : "Assistant"}: ${entry.content}`)
        .join("\n"),
    );
  }

  function downloadHistory() {
    const payload = messages
      .map((entry) => `${entry.role === "user" ? "You" : "Assistant"}: ${entry.content}`)
      .join("\n");
    const blob = new Blob([payload], { type: "text/plain;charset=utf-8" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = `${iface.id}-chat.txt`;
    link.click();
    URL.revokeObjectURL(link.href);
  }

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Chat</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-6">
        <div
          aria-label="Chat transcript"
          role="log"
          className="flex min-h-72 flex-col gap-3 rounded-[1.25rem] border bg-muted/35 p-4"
        >
          {messages.length === 0 ? (
            <p className="text-sm leading-6 text-muted-foreground">Send a message to start the conversation.</p>
          ) : (
            messages.map((item, index) => (
              <div
                key={`${item.role}-${index}`}
                className={
                  item.role === "user"
                    ? "ml-auto max-w-[85%] rounded-3xl rounded-br-md bg-primary px-4 py-3 text-sm leading-6 text-primary-foreground"
                    : "mr-auto max-w-[85%] rounded-3xl rounded-bl-md border bg-card px-4 py-3 text-sm leading-6 shadow-sm"
                }
              >
                {item.content || (item.role === "assistant" && isSubmitting ? "..." : "")}
              </div>
            ))
          )}
        </div>
        {error ? <ErrorAlert title="Stream failed" message={error} /> : null}
        <Field>
          <FieldLabel htmlFor={input.id}>{input.label}</FieldLabel>
          <Textarea
            id={input.id}
            aria-label={input.label}
            disabled={isSubmitting}
            rows={3}
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                void submitMessage();
              }
            }}
          />
        </Field>
      </CardContent>
      <CardFooter className="grid gap-3 border-t bg-card/55 sm:grid-cols-[repeat(5,minmax(0,auto))]">
        <Button type="button" disabled={isSubmitting || message.trim() === ""} onClick={() => void submitMessage()}>
          {isSubmitting ? <Spinner /> : <Send data-icon="inline-start" />}
          {isSubmitting ? (streamPhase === "queued" ? "Queued" : "Running") : "Send"}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!isSubmitting}
          onClick={() => void stopGeneration()}
        >
          Stop
        </Button>
        <Button type="button" variant="outline" onClick={copyHistory}>
          Copy
        </Button>
        <Button type="button" variant="outline" onClick={downloadHistory}>
          Download
        </Button>
        <Button type="button" variant="secondary" onClick={clearHistory}>
          Clear history
        </Button>
      </CardFooter>
    </Card>
  );
}

function VoiceInterface({ iface }: { iface: InterfaceSchema }) {
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

function SchemaInput({
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
  if (props.visible === false) {
    return null;
  }

  if (isLayoutComponent(component.type)) {
    return null;
  }

  const defaultValue = value ?? props.default ?? "";

  if (component.type === "number" || component.type === "slider") {
    const NumericInput = component.type === "slider" ? Slider : Input;
    return (
      <Field data-disabled={disabled || undefined}>
        <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
        <NumericInput
          id={component.id}
          aria-label={component.label}
          disabled={disabled || props.disabled === true}
          type="number"
          min={numberProp(props.min)}
          max={numberProp(props.max)}
          step={numberProp(props.step)}
          value={String(defaultValue)}
          onChange={(event) => onChange(Number(event.target.value))}
        />
      </Field>
    );
  }

  if (component.type === "checkbox") {
    return (
      <Field data-disabled={disabled || undefined}>
        <label className="flex items-center gap-2 text-sm font-medium">
          <Checkbox
            aria-label={component.label}
            checked={Boolean(value ?? props.default ?? false)}
            disabled={disabled || props.disabled === true}
            onChange={(event) => onChange(event.target.checked)}
          />
          {component.label}
        </label>
      </Field>
    );
  }

  if (component.type === "dropdown") {
    return (
      <Field data-disabled={disabled || undefined}>
        <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
        <Select
          id={component.id}
          aria-label={component.label}
          disabled={disabled || props.disabled === true}
          value={String(defaultValue)}
          onChange={(event) => onChange(event.target.value)}
        >
          {(component.choices ?? []).map((choice) => (
            <option key={choice} value={choice}>
              {choice}
            </option>
          ))}
        </Select>
      </Field>
    );
  }

  if (component.type === "audio") {
    return <AudioInput component={component} disabled={disabled} value={value} onChange={onChange} />;
  }

  if (component.type === "file" || component.type === "image") {
    return <FileInput component={component} disabled={disabled} value={value} onChange={onChange} />;
  }

  return (
    <Field data-disabled={disabled || undefined}>
      <FieldLabel htmlFor={component.id}>{component.label}</FieldLabel>
      <Textarea
        id={component.id}
        aria-label={component.label}
        disabled={disabled || props.disabled === true}
        placeholder={stringProp(props.placeholder)}
        rows={numberProp(props.rows) ?? 4}
        value={String(defaultValue)}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

function renderSchemaInputs(
  components: ComponentSchema[],
  disabled: boolean,
  values: Values,
  onChange: (component: ComponentSchema, value: unknown) => void,
) {
  return components.map((component) => {
    if (isLayoutComponent(component.type)) {
      return (
        <LayoutBlock key={component.id} type={component.type} label={component.label}>
          {renderSchemaInputs(component.items ?? [], disabled, values, onChange)}
        </LayoutBlock>
      );
    }

    return (
      <SchemaInput
        key={component.id}
        component={component}
        disabled={disabled}
        value={values[component.id]}
        onChange={(value) => onChange(component, value)}
      />
    );
  });
}

function renderSchemaOutputs(
  components: ComponentSchema[],
  outputs: Outputs,
): { key: string; component: ComponentSchema; value: unknown }[] {
  const rows: { key: string; component: ComponentSchema; value: unknown }[] = [];

  for (const component of components) {
    if (isLayoutComponent(component.type)) {
      if (component.items?.length) {
        rows.push(...renderSchemaOutputs(component.items, outputs));
      }
      continue;
    }

    rows.push({
      key: component.id,
      component,
      value: outputs[component.id],
    });
  }

  return rows;
}

function LayoutBlock({
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

function isLayoutComponent(type: string) {
  return type === "row" || type === "column" || type === "group";
}

function flattenLeafComponents(components: ComponentSchema[]): ComponentSchema[] {
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

function AudioInput({
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

function FileInput({
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

function OutputBlock({ component, value }: { component: ComponentSchema; value: unknown }) {
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

function AudioPreview({ label, asset }: { label: string; asset: UploadResponse }) {
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

function ErrorAlert({ title, message }: { title: string; message: string }) {
  return (
    <Alert variant="destructive">
      <AlertCircle aria-hidden="true" className="mb-2" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}

function useInitialValues(components: ComponentSchema[]) {
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

function readDemoMode(): DemoMode {
  if (typeof window === "undefined") {
    return "none";
  }

  const demo = new URLSearchParams(window.location.search).get("demo");
  switch (demo) {
    case "readme-hero":
    case "readme-components":
    case "readme-outputs":
    case "readme-chat":
    case "readme-adapters":
    case "readme-mobile":
      return demo;
    default:
      return "none";
  }
}

function initialOutputs(components: ComponentSchema[], demoMode: DemoMode): Outputs {
  if (
    demoMode !== "readme-hero" &&
    demoMode !== "readme-outputs" &&
    demoMode !== "readme-adapters" &&
    demoMode !== "readme-mobile"
  ) {
    return {};
  }

  return Object.fromEntries(
    components.flatMap((component) => {
      const seededValue = component.props?.default;
      return seededValue === undefined ? [] : [[component.id, seededValue]];
    }),
  );
}

function normalizeStreamValue(value: unknown): string {
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

function initialChatMessages(demoMode: DemoMode): ChatMessage[] {
  if (demoMode !== "readme-chat") {
    return [];
  }

  return [
    {
      role: "user",
      content: "Need a cleaner launch message for an internal support copilot.",
    },
    {
      role: "assistant",
      content:
        "Start with the outcome your team cares about: faster answers, fewer repetitive tickets, and a rollout plan that support leads can skim in under a minute.",
    },
  ];
}

function outputShowcaseInputs(components: ComponentSchema[]) {
  if (components.length < 4) {
    return components;
  }

  const first = components[0];
  const last = components[components.length - 1];
  const dropdown = components.find((component) => component.type === "dropdown");

  return [first, dropdown, last].filter((component): component is ComponentSchema => component !== undefined);
}

function useAudioRecorder({
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

function useStreamingAudioRecorder({
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

function isUploadResponse(value: unknown): value is UploadResponse {
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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function stringProp(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function numberProp(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}

function supportsAudioRecording() {
  return typeof window !== "undefined" && typeof MediaRecorder !== "undefined" && Boolean(navigator.mediaDevices?.getUserMedia);
}

function extensionFromMime(mimeType: string) {
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

async function blobToBase64(blob: Blob) {
  const buffer =
    typeof blob.arrayBuffer === "function" ? await blob.arrayBuffer() : await new Response(blob).arrayBuffer();
  let binary = "";
  const bytes = new Uint8Array(buffer);
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}
