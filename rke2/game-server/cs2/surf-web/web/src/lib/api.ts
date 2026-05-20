export interface PlayerSummary {
  steam_id: string;
  name: string;
  points: number;
  runs: number;
  rank: number | null;
  avatar: string | null;
  profile_url: string | null;
}

export interface PlayerDetail extends PlayerSummary {
  updated_at: string;
  map_count: number;
  wr_count: number;
}

export interface MapSummary {
  map_id: number;
  file: string;
  tier: number;
  stages: number;
  base_pot: number;
  completions: number;
  wr_holder: string | null;
  wr_time: number | null;
}

export interface RunRecord {
  run_id: number;
  map_id: number;
  map_file: string;
  steam_id: string;
  player_name: string;
  run_type: number;
  track: number;
  stage: number;
  style: number;
  time: number;
  jumps: number;
  strafes: number;
  sync: number;
  date: string;
  avatar: string | null;
}

export interface TrackTop {
  track: number;
  stage: number;
  run_type: number;
  times: RunRecord[];
}

export interface MapDetail {
  map: MapSummary;
  main_top: RunRecord[];
  stages: TrackTop[];
  bonuses: TrackTop[];
}

export interface PlayerProfile {
  player: PlayerDetail;
  personal_bests: RunRecord[];
  recent_runs: RunRecord[];
}

export interface ServerInfo {
  counts: { players: number; runs: number; maps: number };
}

async function get<T>(path: string): Promise<T> {
  const r = await fetch(path, { headers: { Accept: 'application/json' } });
  if (!r.ok) throw new Error(`${path}: ${r.status} ${r.statusText}`);
  return r.json();
}

export const api = {
  server: () => get<ServerInfo>('/api/server'),
  players: (params: { limit?: number; offset?: number; search?: string } = {}) => {
    const q = new URLSearchParams();
    if (params.limit) q.set('limit', String(params.limit));
    if (params.offset) q.set('offset', String(params.offset));
    if (params.search) q.set('search', params.search);
    const qs = q.toString();
    return get<PlayerSummary[]>(`/api/players${qs ? '?' + qs : ''}`);
  },
  player: (steamId: string) => get<PlayerProfile>(`/api/players/${encodeURIComponent(steamId)}`),
  maps: () => get<MapSummary[]>('/api/maps'),
  map: (file: string) => get<MapDetail>(`/api/maps/${encodeURIComponent(file)}`),
  recent: (limit = 25) => get<RunRecord[]>(`/api/records/recent?limit=${limit}`),
  wrs: (limit = 25) => get<RunRecord[]>(`/api/records/wr?limit=${limit}`),
};
