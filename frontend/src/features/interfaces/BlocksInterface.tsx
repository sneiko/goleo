import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { sendEvent } from "@/lib/api";
import type { ComponentSchema, EventSchema, InterfaceSchema } from "@/types";
import {
  applyComponentRuntime,
  blockEventPayload,
  componentRuntimeUpdate,
  hiddenBlockComponentIDs,
  hiddenEventInputIDs,
  isComponentUpdate,
  isEditableBlocksComponent,
  type ComponentRuntimeState,
} from "@/features/blocks/blocks-runtime";
import { LayoutBlock } from "@/features/schema/LayoutBlock";
import { OutputBlock } from "@/features/schema/OutputBlock";
import { SchemaInput } from "@/features/schema/SchemaInput";
import {
  errorMessage,
  flattenLeafComponents,
  isLayoutComponent,
  type Values,
  useInitialValues,
} from "@/features/schema/schema-utils";

export function BlocksInterface({ iface }: { iface: InterfaceSchema }) {
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
