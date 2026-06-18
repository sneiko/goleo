export type ComponentSchema = {
  id: string;
  type: string;
  label: string;
  props?: Record<string, unknown>;
  choices?: string[];
  items?: ComponentSchema[];
};

export type EventSchema = {
  id: string;
  trigger: string;
  source?: string;
  inputs: string[];
  outputs: string[];
};

export type InterfaceSchema = {
  id: string;
  kind: "interface" | "chat" | "voice" | string;
  inputs: ComponentSchema[];
  outputs: ComponentSchema[];
  components?: ComponentSchema[];
  events?: EventSchema[];
};

export type AppSchema = {
  version: string;
  interfaces: InterfaceSchema[];
};

export type ComponentUpdate = {
  kind: "update";
  value?: unknown;
  visible?: boolean;
  disabled?: boolean;
  choices?: string[];
  label?: string;
};

export type EventComponentValue =
  | string
  | number
  | boolean
  | null
  | unknown[]
  | Record<string, unknown>
  | ComponentUpdate;

export type EventResponseData = Record<string, EventComponentValue>;

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
  status?: string;
  text?: string;
  state?: string;
  audio?: UploadResponse;
  progress?: {
    current?: number;
    total?: number;
    message?: string;
  };
};

export type StreamEvent = {
  event: string;
  status?: string;
  data?: unknown;
  request_id?: string;
  error?: string;
  progress?: {
    current?: number;
    total?: number;
    message?: string;
  };
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
