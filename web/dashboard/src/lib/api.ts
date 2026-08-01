import { sanitizePassword, sanitizeUsername } from "@/lib/sanitize";

const API_BASE = "/api";

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export interface AuthUser {
  id?: number;
  username: string;
  must_change_password: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface LoginResponse {
  token: string;
  must_change_password: boolean;
  user: AuthUser;
}

export interface MeResponse {
  user: AuthUser;
}

export type AgentPlatform = "linux" | "windows" | "darwin";
export type AgentDeviceClass = "vps" | "desktop" | "unknown";
export type AgentNetworkType = "datacenter" | "residential" | "unknown";

export interface ProxyNode {
  id: string;
  ip: string;
  port: number;
  username: string;
  country: string;
  region: string;
  city: string;
  zip: string;
  platform?: AgentPlatform | "unknown" | string;
  device_class?: AgentDeviceClass | string;
  network_type?: AgentNetworkType | string;
  is_online: boolean;
  is_healthy: boolean;
  last_seen: string;
  created_at: string;
  updated_at: string;
  last_probe_at?: string;
}

export interface NodesResponse {
  nodes: ProxyNode[];
  count: number;
}

export interface NodeCredentials {
  id: string;
  ip: string;
  port: number;
  username: string;
  password: string;
  connection_string: string;
}

export interface DashboardStats {
  total_agents: number;
  online: number;
  offline: number;
  unhealthy: number;
  probe_failures: number;
  nodes_online: number;
  system_health: "healthy" | "degraded" | "critical";
  status_breakdown: Record<string, number>;
  platform_breakdown: Record<string, number>;
  device_class_breakdown: Record<string, number>;
  country_breakdown: Record<string, number>;
  recent_nodes: ProxyNode[];
}

export interface BootstrapScriptResponse {
  script: string;
  controller_url: string;
  has_agent_key: boolean;
}

export interface DeployPlatform {
  id: string;
  label: string;
  description: string;
  command: string;
  controller_url: string;
  run_as?: string;
  prerequisites?: string;
}

export interface DeployCommandsResponse {
  has_agent_key: boolean;
  ssl_mode: SSLMode;
  public_domain: string;
  production_controller_url: string;
  local_controller_url: string;
  platforms: DeployPlatform[];
}

export type SSLMode =
  | "caddy"
  | "dev-mkcert"
  | "none";

export interface DeploymentConfig {
  public_domain: string;
  controller_public_url: string;
  ssl_mode: SSLMode;
  has_agent_key: boolean;
}

export interface DNSRecord {
  type: string;
  name: string;
  value: string;
  notes?: string;
}

export interface DNSHints {
  domain: string;
  records: DNSRecord[];
  summary: string;
}

export interface DevSetup {
  public_domain: string;
  suggested_controller_url: string;
  hosts_file_entry: string;
  mkcert_instructions: string;
  controller_note: string;
}

export interface CloudflareSetupStep {
  title: string;
  description: string;
}

export interface CloudflareSetup {
  domain: string;
  api_host: string;
  server_ip: string;
  token_url: string;
  token_steps: CloudflareSetupStep[];
  dns_records: DNSRecord[];
  summary: string;
  renewal_note: string;
}

async function parseError(res: Response): Promise<string> {
  try {
    const data = (await res.json()) as {
      error?: string;
      message?: string;
    };
    return data.error ?? data.message ?? res.statusText;
  } catch {
    return res.statusText || "Request failed";
  }
}

function authHeaders(token: string | null): HeadersInit {
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
}

export async function apiFetch<T>(
  path: string,
  token: string | null,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const bearer = authHeaders(token);
  for (const [key, value] of Object.entries(bearer)) {
    headers.set(key, value as string);
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  if (!res.ok) {
    throw new ApiError(await parseError(res), res.status);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

export async function loginRequest(
  username: string,
  password: string,
): Promise<LoginResponse> {
  const cleanUsername = sanitizeUsername(username);
  const cleanPassword = sanitizePassword(password);

  const res = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      username: cleanUsername,
      password: cleanPassword,
    }),
  });

  if (!res.ok) {
    throw new ApiError(await parseError(res), res.status);
  }

  return res.json() as Promise<LoginResponse>;
}

export async function changePasswordRequest(
  currentPassword: string,
  newPassword: string,
  token: string | null,
): Promise<{ user: AuthUser }> {
  return apiFetch("/auth/change-password", token, {
    method: "POST",
    body: JSON.stringify({
      current_password: sanitizePassword(currentPassword),
      new_password: sanitizePassword(newPassword),
    }),
  });
}

export async function meRequest(token: string | null): Promise<MeResponse | null> {
  const res = await fetch(`${API_BASE}/auth/me`, {
    method: "GET",
    credentials: "include",
    headers: authHeaders(token),
  });

  if (res.status === 401) {
    return null;
  }

  if (!res.ok) {
    throw new ApiError(await parseError(res), res.status);
  }

  return res.json() as Promise<MeResponse>;
}

export async function logoutRequest(token: string | null): Promise<void> {
  await fetch(`${API_BASE}/auth/logout`, {
    method: "POST",
    credentials: "include",
    headers: authHeaders(token),
  });
}

export function fetchDashboardStats(token: string | null): Promise<DashboardStats> {
  return apiFetch("/dashboard/stats", token);
}

export function fetchDashboardNodes(token: string | null): Promise<NodesResponse> {
  return apiFetch("/dashboard/nodes", token);
}

export function fetchNodeCredentials(
  token: string | null,
  nodeId: string,
): Promise<NodeCredentials> {
  return apiFetch(`/dashboard/nodes/${encodeURIComponent(nodeId)}/credentials`, token);
}

export function fetchBootstrapScript(
  token: string | null,
): Promise<BootstrapScriptResponse> {
  return apiFetch("/dashboard/bootstrap-script", token);
}

export function fetchDeployCommands(
  token: string | null,
): Promise<DeployCommandsResponse> {
  return apiFetch("/dashboard/deploy-commands", token);
}

export function fetchDeployment(token: string | null): Promise<DeploymentConfig> {
  return apiFetch("/dashboard/deployment", token);
}

export function updateDeployment(
  token: string | null,
  payload: Pick<DeploymentConfig, "public_domain" | "controller_public_url" | "ssl_mode">,
): Promise<DeploymentConfig> {
  return apiFetch("/dashboard/deployment", token, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export function regenerateAgentKey(
  token: string | null,
): Promise<{ status: string; has_agent_key: boolean; message: string }> {
  return apiFetch("/dashboard/deployment/regenerate-agent-key", token, {
    method: "POST",
  });
}

export interface DNSHintsQuery {
  domain?: string;
  ssl_mode?: SSLMode;
}

export function fetchDNSHints(
  token: string | null,
  query?: DNSHintsQuery,
): Promise<DNSHints> {
  const params = new URLSearchParams();
  if (query?.domain) params.set("domain", query.domain);
  if (query?.ssl_mode) params.set("ssl_mode", query.ssl_mode);
  const qs = params.toString();
  const path = qs
    ? `/dashboard/deployment/dns-hints?${qs}`
    : "/dashboard/deployment/dns-hints";
  return apiFetch(path, token);
}

export function fetchDevSetup(token: string | null): Promise<DevSetup> {
  return apiFetch("/dashboard/deployment/dev-setup", token);
}

export interface CloudflareSetupQuery {
  domain?: string;
  ssl_mode?: SSLMode;
}

export function fetchCloudflareSetup(
  token: string | null,
  query?: CloudflareSetupQuery,
): Promise<CloudflareSetup> {
  const params = new URLSearchParams();
  if (query?.domain) params.set("domain", query.domain);
  if (query?.ssl_mode) params.set("ssl_mode", query.ssl_mode);
  const qs = params.toString();
  const path = qs
    ? `/dashboard/deployment/cloudflare-setup?${qs}`
    : "/dashboard/deployment/cloudflare-setup";
  return apiFetch(path, token);
}

export function provisionSSL(
  token: string | null,
  domain: string,
  cloudflareApiToken: string,
): Promise<{ status: string; output: string }> {
  return apiFetch("/dashboard/deployment/provision-ssl", token, {
    method: "POST",
    body: JSON.stringify({
      domain,
      cloudflare_api_token: cloudflareApiToken,
    }),
  });
}
