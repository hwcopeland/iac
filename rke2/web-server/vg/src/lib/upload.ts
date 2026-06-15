import { writeFile } from 'node:fs/promises';
import { join, extname } from 'node:path';
import { nanoid } from 'nanoid';
import { UPLOAD_DIR } from '@/db';

const ALLOWED = new Set(['.jpg', '.jpeg', '.png', '.webp', '.gif', '.avif']);
const MAX_BYTES = 8 * 1024 * 1024;

// Persists an uploaded image to the data volume and returns its public URL
// (/uploads/<name>), or null when no file was provided. Throws on bad input.
export async function saveUpload(file: FormDataEntryValue | null): Promise<string | null> {
  if (!file || typeof file === 'string') return null;
  if (file.size === 0) return null;
  if (file.size > MAX_BYTES) throw new Error('File too large (max 8MB)');

  let ext = extname(file.name).toLowerCase();
  if (!ALLOWED.has(ext)) {
    const fromType = '.' + (file.type.split('/')[1] ?? '');
    if (!ALLOWED.has(fromType)) throw new Error('Unsupported image type');
    ext = fromType === '.jpeg' ? '.jpg' : fromType;
  }

  const name = `${nanoid(12)}${ext}`;
  const buf = Buffer.from(await file.arrayBuffer());
  await writeFile(join(UPLOAD_DIR, name), buf);
  return `/uploads/${name}`;
}
