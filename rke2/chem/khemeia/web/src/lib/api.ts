const API_BASE = 'https://khemeia.hwcopeland.net';
let token: string | null = null;

export function setToken(t: string) {
  token = t;
}

export function getToken(): string | null {
  return token;
}

export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || res.statusText);
  }
  return res.json();
}

export interface PluginInputField {
  name: string;
  type: string;
  required?: boolean;
  default?: any;
  description?: string;
  max?: number;
  enum?: string[];
}

export interface PluginOutputField {
  name: string;
  type: string;
}

export interface Plugin {
  name: string;
  slug: string;
  input: PluginInputField[];
  output: PluginOutputField[];
}

export interface PluginsResponse {
  plugins: Plugin[];
}

export async function getPlugins(): Promise<PluginsResponse> {
  return api<PluginsResponse>('/api/v1/plugins');
}

export async function submitJob(slug: string, data: Record<string, any>): Promise<any> {
  return api(`/api/v1/${slug}/submit`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function getJobs(slug: string): Promise<any> {
  return api(`/api/v1/${slug}/jobs`);
}
