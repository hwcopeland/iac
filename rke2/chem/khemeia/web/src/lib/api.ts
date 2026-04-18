const API_BASE = '';
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

export async function getJob(slug: string, jobName: string): Promise<any> {
  return api(`/api/v1/${slug}/jobs/${jobName}`);
}

export async function getLigandDatabases(): Promise<{ databases: { name: string; count: number }[] }> {
  return api('/api/v1/ligand-databases');
}

// --- Artifact API ---

export interface ArtifactSummary {
  id: number;
  filename: string;
  content_type: string;
  size_bytes: number;
  created_at: string;
}

export async function getJobArtifacts(slug: string, jobName: string): Promise<{ artifacts: ArtifactSummary[] }> {
  return api(`/api/v1/${slug}/artifacts/${jobName}`);
}

export function getArtifactUrl(slug: string, jobName: string, filename: string): string {
  return `/api/v1/${slug}/artifacts/${jobName}/${encodeURIComponent(filename)}`;
}

export async function downloadArtifact(slug: string, jobName: string, filename: string): Promise<ArrayBuffer> {
  const url = getArtifactUrl(slug, jobName, filename);
  const headers: Record<string, string> = {};
  const t = getToken();
  if (t) headers['Authorization'] = `Bearer ${t}`;
  const res = await fetch(url, { headers });
  if (!res.ok) throw new Error(`Artifact download failed: ${res.statusText}`);
  return res.arrayBuffer();
}

// --- Ligand Search API ---

export interface Compound {
  chembl_id: string;
  pref_name: string;
  smiles: string;
  mw: number;
  logp: number;
  hba: number;
  hbd: number;
  psa: number;
  ro5_violations: number;
  qed: number;
  max_phase: number;
  formula: string;
}

export interface SearchResponse {
  compounds: Compound[];
  total: number;
  limit: number;
  offset: number;
}

export interface ImportResponse {
  source_db: string;
  imported: number;
}

export async function searchLigands(params: Record<string, string>): Promise<SearchResponse> {
  const qs = new URLSearchParams(params).toString();
  return api<SearchResponse>(`/api/v1/ligands/search?${qs}`);
}

export async function importFromChEMBL(chemblIds: string[], sourceDb: string): Promise<ImportResponse> {
  return api<ImportResponse>('/api/v1/ligands/import-from-chembl', {
    method: 'POST',
    body: JSON.stringify({ chembl_ids: chemblIds, source_db: sourceDb }),
  });
}

export async function importFromFilter(params: Record<string, any>): Promise<ImportResponse> {
  return api<ImportResponse>('/api/v1/ligands/import-from-filter', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

// --- Pocket Analysis ---

export interface PocketResidue {
  chain_id: string;
  res_id: number;
  res_name: string;
  min_distance: number;
  interactions: string[];
  contact_atoms: number;
}

export interface PocketAnalysis {
  compound_id: string;
  cutoff_angstrom: number;
  pocket_residues: PocketResidue[];
  total_contacts: number;
  ligand_atoms: number;
}

export async function getPocketAnalysis(jobName: string, compoundId: string, cutoff?: number): Promise<PocketAnalysis> {
  const params = cutoff ? `?cutoff=${cutoff}` : '';
  return api<PocketAnalysis>(`/api/v1/docking/pocket/${jobName}/${encodeURIComponent(compoundId)}${params}`);
}
