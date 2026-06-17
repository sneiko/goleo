import { useEffect, useMemo, useState } from "react";
import { AlertCircle, FileText, Send } from "lucide-react";
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
import { loadSchema, predict, stream, uploadFile } from "@/lib/api";
import type { AppSchema, ComponentSchema, InterfaceSchema, UploadResponse } from "@/types";

type Values = Record<string, unknown>;
type Outputs = Record<string, unknown>;
type ChatMessage = {
  role: "user" | "assistant";
  content: string;
};

export default function App() {
  const [schema, setSchema] = useState<AppSchema | null>(null);
  const [error, setError] = useState<string | null>(null);

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
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
        <header className="flex flex-col gap-2">
          <p className="text-sm font-medium text-muted-foreground">Goleo</p>
          <h1 className="text-3xl font-semibold tracking-normal sm:text-4xl">AI demos from Go functions</h1>
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
          iface.kind === "chat" ? <ChatInterface key={iface.id} iface={iface} /> : <FormInterface key={iface.id} iface={iface} />,
        )}
      </div>
    </main>
  );
}

function LoadingState() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64" />
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-9 w-28" />
      </CardContent>
    </Card>
  );
}

function FormInterface({ iface }: { iface: InterfaceSchema }) {
  const [values, setValues] = useInitialValues(iface.inputs);
  const [outputs, setOutputs] = useState<Outputs>({});
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIsSubmitting(true);
    try {
      const result = await predict(
        iface.id,
        iface.inputs.map((component) => values[component.id] ?? ""),
      );
      setOutputs(Object.fromEntries(iface.outputs.map((component, index) => [component.id, result[index]])));
    } catch (submitError) {
      setError(errorMessage(submitError));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Interface</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <form onSubmit={onSubmit}>
        <CardContent className="flex flex-col gap-5">
          <FieldGroup>
            {iface.inputs.map((component) => (
              <SchemaInput
                key={component.id}
                component={component}
                disabled={isSubmitting}
                value={values[component.id]}
                onChange={(value) => setValues((current) => ({ ...current, [component.id]: value }))}
              />
            ))}
          </FieldGroup>
          {error ? <ErrorAlert title="Request failed" message={error} /> : null}
          {iface.outputs.length > 0 ? (
            <div className="grid gap-3">
              {iface.outputs.map((component) => (
                <OutputBlock key={component.id} component={component} value={outputs[component.id]} />
              ))}
            </div>
          ) : null}
        </CardContent>
        <CardFooter>
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? <Spinner /> : null}
            {isSubmitting ? "Running" : "Run"}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

function ChatInterface({ iface }: { iface: InterfaceSchema }) {
  const [message, setMessage] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const input = iface.inputs[0] ?? { id: `${iface.id}-message`, type: "textbox", label: "Message", props: {} };

  async function submitMessage() {
    const trimmed = message.trim();
    if (trimmed === "" || isSubmitting) {
      return;
    }

    setError(null);
    setMessage("");
    setIsSubmitting(true);
    setMessages((current) => [...current, { role: "user", content: trimmed }, { role: "assistant", content: "" }]);

    try {
      await stream(iface.id, [trimmed], (chunk) => {
        setMessages((current) => {
          const next = [...current];
          const last = next[next.length - 1];
          if (last?.role === "assistant") {
            next[next.length - 1] = { ...last, content: last.content + chunk };
          }
          return next;
        });
      });
    } catch (streamError) {
      setError(errorMessage(streamError));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Chat</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        <div aria-label="Chat transcript" role="log" className="flex min-h-48 flex-col gap-3 rounded-md border bg-muted/30 p-3">
          {messages.length === 0 ? (
            <p className="text-sm text-muted-foreground">Send a message to start the conversation.</p>
          ) : (
            messages.map((item, index) => (
              <div
                key={`${item.role}-${index}`}
                className={item.role === "user" ? "ml-auto max-w-[85%] rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground" : "mr-auto max-w-[85%] rounded-md bg-card px-3 py-2 text-sm shadow-sm"}
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
      <CardFooter>
        <Button type="button" disabled={isSubmitting || message.trim() === ""} onClick={() => void submitMessage()}>
          {isSubmitting ? <Spinner /> : <Send data-icon="inline-start" />}
          Send
        </Button>
      </CardFooter>
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

  if (component.type === "file" || component.type === "image") {
    return <FileInput component={component} disabled={disabled} onChange={onChange} />;
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

function FileInput({
  component,
  disabled,
  onChange,
}: {
  component: ComponentSchema;
  disabled: boolean;
  onChange: (value: unknown) => void;
}) {
  const props = component.props ?? {};
  const [upload, setUpload] = useState<UploadResponse | null>(null);
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
        <div className="flex items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-sm">
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

  const content = component.type === "json" || typeof value === "object" ? JSON.stringify(value ?? "", null, 2) : String(value ?? "");

  return (
    <section className="rounded-md border bg-muted/30 p-3">
      <h3 className="mb-2 text-sm font-medium">{component.label}</h3>
      <pre className="whitespace-pre-wrap break-words text-sm">{content}</pre>
    </section>
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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function stringProp(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function numberProp(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}
