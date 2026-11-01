// Wire types mirror Controls' JSON API. UI code does not import controller internals.
export interface Usage {
  samples: number;
  unreported_sessions: number;
  cost_samples: number;
  cost_usd: number;
  tokens: {
    input: number;
    output: number;
    cache_read: number;
    cache_write: number;
  };
  model?: string;
  provider?: string;
  tool_failures: number;
}
export interface StepDefinition {
  id: string;
  title?: string;
  stage?: string;
  kind: string;
  routes: Record<string, string>;
  outputs?: string[];
  run?: { package: string };
  decision?: {
    authority: string;
    prompt: string;
    options: string[];
    artifacts?: string[];
  };
}
export interface StageDefinition {
  id: string;
  steps: string[];
  loop?: { mode?: string; limit: number };
  human?: { required: boolean; step?: string };
}
export interface StepVisit {
  id: string;
  key: string;
  stage?: string;
  title?: string;
  visit: number;
  try: number;
  state: string;
  outcome?: string;
  error?: string;
  started_at: string;
  finished_at?: string;
  sessions?: string[];
  result?: { summary: string; artifacts?: { name: string; uri: string }[] };
  resources?: Usage;
}
export interface Decision {
  id: string;
  execution_id: string;
  step_id: string;
  owner?: string;
  kind?: string;
  prompt: string;
  options: string[];
  state: string;
  created_at: string;
  answer?: string;
  reason?: string;
}
export interface Execution {
  id: string;
  context_id: string;
  workflow: string;
  project: string;
  brief?: string;
  state: string;
  error?: string;
  created_at: string;
  updated_at: string;
  finished_at?: string;
  resources?: Usage;
  steps?: StepVisit[];
  requests?: Decision[];
  stages?: {
    name: string;
    visit: number;
    state: string;
    started_at: string;
    finished_at?: string;
  }[];
  hooks?: { id: string; name: string; state: string; error?: string }[];
  configuration: {
    files_dir?: string;
    workflow: {
      name: string;
      start: string;
      stages?: StageDefinition[];
      steps: StepDefinition[];
    };
  };
}
export interface Session {
  id: string;
  name: string;
  state: string;
  input_mode: string;
  runner: string;
  step_id?: string;
  execution_id?: string;
  pid?: number;
  started_at: string;
  ended_at?: string;
  resources?: Usage;
}
export interface Message {
  id: string;
  thread_id: string;
  sequence: number;
  sender: string;
  sender_name: string;
  recipients: string[];
  recipient_names?: string[];
  at: string;
  payload: {
    kind: string;
    version: number;
    parts: (
      | { kind: "text"; text: string }
      | { kind: "data"; data: unknown }
      | { kind: string; [key: string]: unknown }
    )[];
  };
}
export interface SessionMessage {
  id: string;
  session_id: string;
  sender?: string;
  body: string;
  state: string;
  created_at: string;
}
export interface Context {
  id: string;
  description: string;
}
