import type { ComponentSchema, ComponentUpdate, EventSchema } from "@/types";

export type ComponentRuntimeState = {
  visible?: boolean;
  disabled?: boolean;
  choices?: string[];
  label?: string;
};

export type Values = Record<string, unknown>;

export function applyComponentRuntime(component: ComponentSchema, runtime?: ComponentRuntimeState): ComponentSchema {
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

export function blockEventPayload(
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

export function hiddenEventInputIDs(event: EventSchema, hiddenComponentIDs: Set<string>) {
  return event.inputs.filter((componentID) => hiddenComponentIDs.has(componentID));
}

export function hiddenBlockComponentIDs(
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

export function componentRuntimeUpdate(update: ComponentUpdate): ComponentRuntimeState | null {
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

export function isComponentUpdate(value: unknown): value is ComponentUpdate {
  return Boolean(
    value &&
      typeof value === "object" &&
      (value as { __goleo_update__?: unknown }).__goleo_update__ === true &&
      (value as { kind?: unknown }).kind === "update",
  );
}

export function isEditableBlocksComponent(type: string) {
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
