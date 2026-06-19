import { describe, expect, it } from "vitest";
import type { ComponentSchema, EventSchema } from "@/types";
import {
  applyComponentRuntime,
  blockEventPayload,
  componentRuntimeUpdate,
  hiddenBlockComponentIDs,
  hiddenEventInputIDs,
  isComponentUpdate,
} from "./blocks-runtime";

describe("blocks runtime helpers", () => {
  it("excludes hidden inputs from event payloads and reports them separately", () => {
    const event: EventSchema = {
      id: "submit",
      trigger: "click",
      inputs: ["visible", "hidden", "state"],
      outputs: [],
    };
    const componentByID = new Map<string, ComponentSchema>([
      ["visible", { id: "visible", type: "textbox", label: "Visible" }],
      ["hidden", { id: "hidden", type: "textbox", label: "Hidden" }],
      ["state", { id: "state", type: "state", label: "State" }],
    ]);
    const hiddenIDs = new Set(["hidden"]);

    expect(blockEventPayload(event, { visible: "ok", hidden: "secret", state: "internal" }, componentByID, hiddenIDs)).toEqual({
      visible: "ok",
    });
    expect(hiddenEventInputIDs(event, hiddenIDs)).toEqual(["hidden"]);
  });

  it("treats descendants of hidden layout components as hidden", () => {
    const components: ComponentSchema[] = [
      {
        id: "advanced",
        type: "group",
        label: "Advanced",
        props: {},
        items: [{ id: "secret", type: "textbox", label: "Secret" }],
      },
      { id: "visible", type: "textbox", label: "Visible" },
    ];

    expect(hiddenBlockComponentIDs(components, { advanced: { visible: false } })).toEqual(new Set(["advanced", "secret"]));
  });

  it("applies runtime visible disabled choices and label updates", () => {
    const component: ComponentSchema = {
      id: "mode",
      type: "dropdown",
      label: "Mode",
      choices: ["fast"],
      props: { disabled: false },
    };

    expect(
      applyComponentRuntime(component, {
        visible: false,
        disabled: true,
        choices: ["accurate"],
        label: "Runtime mode",
      }),
    ).toEqual({
      id: "mode",
      type: "dropdown",
      label: "Runtime mode",
      choices: ["accurate"],
      props: { disabled: true, visible: false },
    });
    expect(component.props).toEqual({ disabled: false });
  });

  it("converts update envelopes to runtime state and rejects unmarked objects", () => {
    expect(
      componentRuntimeUpdate({
        __goleo_update__: true,
        kind: "update",
        value: "done",
        visible: true,
        disabled: false,
        choices: ["a", "b"],
        label: "Next",
      }),
    ).toEqual({
      visible: true,
      disabled: false,
      choices: ["a", "b"],
      label: "Next",
    });
    expect(isComponentUpdate({ kind: "update", value: "plain" })).toBe(false);
  });
});
