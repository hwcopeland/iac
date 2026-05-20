export function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '--:--.---';
  const m = Math.floor(seconds / 60);
  const s = seconds - m * 60;
  return `${String(m).padStart(2, '0')}:${s.toFixed(3).padStart(6, '0')}`;
}

export function formatRelative(iso: string): string {
  // Accept naive strings as UTC as a defence-in-depth in case the server
  // ever returns a timestamp without an offset.
  const normalized = /[zZ]|[+-]\d{2}:?\d{2}$/.test(iso) ? iso : iso + 'Z';
  const then = new Date(normalized).getTime();
  if (Number.isNaN(then)) return iso;
  const diff = (Date.now() - then) / 1000;
  if (diff < 0) return new Date(normalized).toLocaleString();
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  return new Date(normalized).toLocaleDateString();
}

export function mapDisplayName(file: string): string {
  return file.replace(/^surf_/, '').replace(/_/g, ' ');
}

export function trackLabel(track: number, stage: number, run_type: number): string {
  if (track > 0) return `Bonus ${track}`;
  if (run_type === 1) return `Stage ${stage}`;
  return 'Main';
}

export function rankClass(rank: number): string {
  if (rank === 1) return 'rank gold';
  if (rank === 2) return 'rank silver';
  if (rank === 3) return 'rank bronze';
  return 'rank';
}

export function defaultAvatar(): string {
  // Steam default avatar
  return 'https://avatars.steamstatic.com/fef49e7fa7e1997310d705b2a6158ff8dc1cdfeb_full.jpg';
}
