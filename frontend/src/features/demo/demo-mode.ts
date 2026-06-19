import type { ComponentSchema } from "@/types";
import type { Outputs } from "@/features/schema/schema-utils";

export type DemoMode =
  | "none"
  | "readme-hero"
  | "readme-components"
  | "readme-outputs"
  | "readme-chat"
  | "readme-adapters"
  | "readme-mobile";

export type ChatMessage = {
  role: "user" | "assistant";
  content: string;
};

export function readDemoMode(): DemoMode {
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

export function initialOutputs(components: ComponentSchema[], demoMode: DemoMode): Outputs {
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

export function initialChatMessages(demoMode: DemoMode): ChatMessage[] {
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

export function outputShowcaseInputs(components: ComponentSchema[]) {
  if (components.length < 4) {
    return components;
  }

  const first = components[0];
  const last = components[components.length - 1];
  const dropdown = components.find((component) => component.type === "dropdown");

  return [first, dropdown, last].filter((component): component is ComponentSchema => component !== undefined);
}
