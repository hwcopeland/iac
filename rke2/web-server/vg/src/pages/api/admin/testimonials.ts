import type { APIRoute } from 'astro';
import { eq } from 'drizzle-orm';
import { db } from '@/db';
import { testimonials } from '@/db/schema';

export const prerender = false;

const num = (v: FormDataEntryValue | null, d = 0) => {
  const n = parseInt(String(v ?? ''), 10);
  return Number.isFinite(n) ? n : d;
};

export const POST: APIRoute = async ({ request, redirect }) => {
  const form = await request.formData();
  const action = String(form.get('_action') ?? '');
  const id = num(form.get('id'));

  if (action === 'delete') {
    await db.delete(testimonials).where(eq(testimonials.id, id));
    return redirect('/admin/testimonials?saved=1');
  }

  const values = {
    quote: String(form.get('quote') ?? '').trim(),
    author: String(form.get('author') ?? '').trim(),
    sortOrder: num(form.get('sortOrder')),
  };
  if (!values.quote) return redirect('/admin/testimonials?err=Quote+is+required');

  if (action === 'update') {
    await db.update(testimonials).set(values).where(eq(testimonials.id, id));
  } else {
    await db.insert(testimonials).values(values);
  }
  return redirect('/admin/testimonials?saved=1');
};
