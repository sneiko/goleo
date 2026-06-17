export type ComponentSchema = {
  id: string;
  type: string;
  label: string;
  props?: Record<string, unknown>;
  choices?: string[];
};

export type InterfaceSchema = {
  id: string;
  kind: "interface" | "chat" | string;
  inputs: ComponentSchema[];
  outputs: ComponentSchema[];
};

export type AppSchema = {
  version: string;
  interfaces: InterfaceSchema[];
};

export type UploadResponse = {
  id: string;
  name: string;
  size: number;
  content_type: string;
};
