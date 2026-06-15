import type { APIRoute } from 'astro';
import { eq } from 'drizzle-orm';
import { db } from '@/db';
import { settings } from '@/db/schema';

export const prerender = false;

const FIELDS = [
  'businessName', 'tagline', 'phone', 'email', 'address', 'facebookUrl',
  'hoursWeekday', 'hoursSaturday', 'hoursSunday', 'heroHeading',
  'heroSubheading', 'homeIntro', 'aboutTitle', 'aboutBody',
] as const;

export const POST: APIRoute = async ({ request, redirect }) => {
  const form = await request.formData();
  const values: Record<string, string> = {};
  for (const key of FIELDS) values[key] = String(form.get(key) ?? '').trim();
  await db.update(settings).set(values).where(eq(settings.id, 1));
  return redirect('/admin/settings?saved=1');
};
