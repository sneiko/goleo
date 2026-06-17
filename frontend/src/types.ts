export type ComponentSchema = {
  id: string;
  type: string;
  label: string;
  props?: Record<string, unknown>;
  choices?: string[];
};

export type InterfaceSchema = {
  id: string;
  kind: "interface" | "chat" | "voice" | string;
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
  url?: string;
};

export type VoiceClientAudio = {
  mime_type: string;
  sequence: number;
  data: string;
};

export type VoiceClientEvent = {
  type: string;
  text?: string;
  state?: string;
  audio?: VoiceClientAudio;
};

export type VoiceServerEvent = {
  type: string;
  text?: string;
  state?: string;
  audio?: UploadResponse;
};

export type VoiceSessionCallbacks = {
  onEvent: (event: VoiceServerEvent) => void;
  onError: (error: Error) => void;
  onClose: () => void;
};

export type VoiceSessionConnection = {
  send: (event: VoiceClientEvent) => void;
  close: () => void;
};
