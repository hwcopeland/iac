import type { APIRoute } from 'astro';
import { eq } from 'drizzle-orm';
import { db } from '@/db';
import { teamMembers } from '@/db/schema';
import { saveUpload } from '@/lib/upload';

export const prerender = false;

const num = (v: FormDataEntryValue | null, d = 0) => {
  const n = parseInt(String(v ?? ''), 10);
  return Number.isFinite(n) ? n : d;
};

export const POST: APIRoute = async ({ request, redirect }) => {
  try {
    const form = await request.formData();
    const action = String(form.get('_action') ?? '');
    const id = num(form.get('id'));

    if (action === 'delete') {
      await db.delete(teamMembers).where(eq(teamMembers.id, id));
      return redirect('/admin/team?saved=1');
    }

    const uploaded = await saveUpload(form.get('photo'));
    const photoUrl = uploaded ?? String(form.get('photoUrl') ?? '').trim();

    const values = {
      name: String(form.get('name') ?? '').trim(),
      role: String(form.get('role') ?? '').trim(),
      tenure: String(form.get('tenure') ?? '').trim(),
      favoritePlant: String(form.get('favoritePlant') ?? '').trim(),
      bio: String(form.get('bio') ?? '').trim(),
      photoPosition: String(form.get('photoPosition') ?? '').trim() || '50% 20%',
      sortOrder: num(form.get('sortOrder')),
    };
    if (!values.name) return redirect('/admin/team?err=Name+is+required');

    if (action === 'update') {
      await db
        .update(teamMembers)
        .set({ ...values, ...(photoUrl ? { photoUrl } : {}) })
        .where(eq(teamMembers.id, id));
    } else {
      await db.insert(teamMembers).values({ ...values, photoUrl });
    }
    return redirect('/admin/team?saved=1');
  } catch (e) {
    return redirect(`/admin/team?err=${encodeURIComponent((e as Error).message)}`);
  }
};
