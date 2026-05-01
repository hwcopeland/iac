const API_BASE = '';
let token: string | null = null;

export function setToken(t: string) {
  token = t;
}

export function getToken(): string | null {
  return token;
}

export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const doFetch = async (): Promise<Response> => {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options?.headers as Record<string, string>),
    };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    return fetch(`${API_BASE}${path}`, { ...options, headers });
  };

  let res = await doFetch();

  // On 401, attempt a silent token refresh and retry once
  if (res.status === 401 && token) {
    try {
      // Dynamically import to avoid circular dependency
      const { restoreSession } = await import('./auth');
      await restoreSession();
      res = await doFetch();
    } catch {
      // Refresh failed — fall through to the error handling below
    }
  }

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

export async function getJob(slug: string, jobName: string, page?: number, perPage?: number): Promise<any> {
  const params = new URLSearchParams();
  if (page) params.set('page', String(page));
  if (perPage) params.set('per_page', String(perPage));
  const qs = params.toString();
  return api(`/api/v1/${slug}/jobs/${jobName}${qs ? '?' + qs : ''}`);
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

export interface InteractionLine {
  type: string;
  rec_x: number; rec_y: number; rec_z: number;
  lig_x: number; lig_y: number; lig_z: number;
  distance: number;
  rec_atom: string;
  lig_atom: string;
  res_name: string;
  res_id: number;
  chain_id: string;
}

export interface PocketAnalysis {
  compound_id: string;
  cutoff_angstrom: number;
  pocket_residues: PocketResidue[];
  interaction_lines: InteractionLine[];
  total_contacts: number;
  ligand_atoms: number;
}

export async function getPocketAnalysis(jobName: string, compoundId: string, cutoff?: number): Promise<PocketAnalysis> {
  const params = cutoff ? `?cutoff=${cutoff}` : '';
  return api<PocketAnalysis>(`/api/v1/docking/pocket/${jobName}/${encodeURIComponent(compoundId)}${params}`);
}

// --- Docking Analysis ---

export interface ResidueContact {
  chain_id: string;
  res_id: number;
  res_name: string;
  contact_frequency: number;
  influence_score: number;
  avg_distance: number;
  interaction_counts: Record<string, number>;
  compounds_contacting: number;
}

export interface ReceptorContactsResponse {
  job_name: string;
  top_n: number;
  residue_contacts: ResidueContact[];
  total_compounds_analyzed: number;
}

export interface ADMETFlags {
  lipinski: boolean;
  veber: boolean;
  lead_like: boolean;
  good_qed: boolean;
  p450_risk: boolean;
  high_psa: boolean;
}

export interface FingerprintCompound {
  compound_id: string;
  smiles: string;
  affinity: number;
  mw?: number;
  logp?: number;
  hba?: number;
  hbd?: number;
  psa?: number;
  ro5_violations?: number;
  qed?: number;
  admet?: ADMETFlags;
}

export interface FingerprintsResponse {
  job_name: string;
  compounds: FingerprintCompound[];
  total: number;
}

export async function getLigandSmiles(compoundId: string): Promise<string | null> {
  try {
    const res = await searchLigands({ q: compoundId, limit: '1' });
    const match = res.compounds?.find((c: any) => c.chembl_id === compoundId);
    return match?.smiles ?? null;
  } catch { return null; }
}

export async function getInteractionMap(jobName: string, compoundId: string): Promise<{ html: string; svg: string; interactions: any[] }> {
  return api(`/api/v1/docking/interaction-map/${jobName}/${encodeURIComponent(compoundId)}`);
}

export async function getReceptorContacts(jobName: string, top?: number): Promise<ReceptorContactsResponse> {
  const params = top ? `?top=${top}` : '';
  return api<ReceptorContactsResponse>(`/api/v1/docking/analysis/receptor-contacts/${jobName}${params}`);
}

export async function getFingerprints(jobName: string, top?: number): Promise<FingerprintsResponse> {
  const params = top ? `?top=${top}` : '';
  return api<FingerprintsResponse>(`/api/v1/docking/analysis/fingerprints/${jobName}${params}`);
}

export async function getDockingResult(jobName: string, compoundId: string): Promise<any> {
  const res = await api<any>(`/api/v1/docking/jobs/${jobName}?compound=${encodeURIComponent(compoundId)}`);
  return res?.docking_results?.[0] ?? null;
}

// --- WP-1: Target Preparation ---

export async function submitTargetPrep(params: any): Promise<any> {
  return api('/api/v1/targets/prepare', { method: 'POST', body: JSON.stringify(params) });
}

export async function getTargetPrep(name: string): Promise<any> {
  return api(`/api/v1/targets/${name}`);
}

export async function getTargetPockets(name: string): Promise<any> {
  return api(`/api/v1/targets/${name}/pockets`);
}

export async function selectPocket(name: string, index: number): Promise<any> {
  return api(`/api/v1/targets/${name}/pockets/${index}/select`, { method: 'POST' });
}

// --- WP-2: Library Preparation ---

export async function submitLibraryPrep(params: any): Promise<any> {
  return api('/api/v1/libraries/prepare', { method: 'POST', body: JSON.stringify(params) });
}

export async function getLibraryPrep(name: string): Promise<any> {
  return api(`/api/v1/libraries/${name}`);
}

export async function getLibraryCompounds(name: string, page?: number, perPage?: number): Promise<any> {
  const p = new URLSearchParams();
  if (page) p.set('page', String(page));
  if (perPage) p.set('per_page', String(perPage));
  return api(`/api/v1/libraries/${name}/compounds?${p}`);
}

// --- WP-3: Docking V2 ---

export async function submitDocking(params: any): Promise<any> {
  return api('/api/v1/docking/v2/submit', { method: 'POST', body: JSON.stringify(params) });
}

export async function getDockingV2Job(name: string): Promise<any> {
  return api(`/api/v1/docking/v2/jobs/${name}`);
}

export async function getDockingV2Results(name: string, page?: number, perPage?: number): Promise<any> {
  const p = new URLSearchParams();
  if (page) p.set('page', String(page));
  if (perPage) p.set('per_page', String(perPage));
  return api(`/api/v1/docking/v2/jobs/${name}/results?${p}`);
}

// --- WP-4: ADMET Predictions ---

export async function submitADMET(params: any): Promise<any> {
  return api('/api/v1/admet/predict', { method: 'POST', body: JSON.stringify(params) });
}

export async function getADMETJob(name: string): Promise<any> {
  return api(`/api/v1/admet/jobs/${name}`);
}

export async function getADMETResults(name: string, page?: number, perPage?: number): Promise<any> {
  const p = new URLSearchParams();
  if (page) p.set('page', String(page));
  if (perPage) p.set('per_page', String(perPage));
  return api(`/api/v1/admet/jobs/${name}/results?${p}`);
}

export async function getADMETCompound(compoundId: string): Promise<any> {
  return api(`/api/v1/admet/compound/${encodeURIComponent(compoundId)}`);
}

// --- WP-5: MD Simulation ---

export async function submitMD(params: any): Promise<any> {
  return api('/api/v1/md/submit', { method: 'POST', body: JSON.stringify(params) });
}

export async function getMDJob(name: string): Promise<any> {
  return api(`/api/v1/md/jobs/${name}`);
}

export async function getMDResults(name: string): Promise<any> {
  return api(`/api/v1/md/jobs/${name}/results`);
}

// --- WP-9: Stage Advance ---

export async function advanceStage(kind: string, name: string, body: any): Promise<any> {
  return api(`/api/v1/jobs/${kind}/${name}/advance`, { method: 'POST', body: JSON.stringify(body) });
}
