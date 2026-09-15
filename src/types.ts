export type ApprovalMode = "always" | "never";
export type PluginKind = "remote_mcp" | "connector";

export interface Skill {
  id: string;
  name: string;
  description: string;
  triggers: string[];
  plugins: string[];
  instructions: string;
}

export interface Plugin {
  id: string;
  label: string;
  description: string;
  kind: PluginKind;
  serverUrl?: string;
  connectorId?: string;
  authorizationEnv?: string;
  allowedTools: string[];
  approval: ApprovalMode;
  enabled: boolean;
  trusted: boolean;
}

export interface TraceEvent {
  id: string;
  at: string;
  type: "route" | "skill" | "plugin" | "tools" | "approval" | "result" | "answer" | "error";
  title: string;
  detail: string;
  status: "neutral" | "waiting" | "success" | "error";
}

export interface AgentReply {
  sessionId: string;
  mode: "live" | "simulation";
  text: string | null;
  skill: Pick<Skill, "id" | "name" | "description">;
  approval: ApprovalRequest | null;
  trace: TraceEvent[];
}

export interface ApprovalRequest {
  id: string;
  plugin: string;
  tool: string;
  arguments: string;
}

export interface Session {
  id: string;
  previousResponseId?: string;
  messageCount: number;
  activePluginIds: string[];
  pendingApproval?: ApprovalRequest;
  pendingResponseId?: string;
}
