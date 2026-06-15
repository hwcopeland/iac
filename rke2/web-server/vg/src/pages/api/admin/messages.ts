import type { APIRoute } from 'astro';
import { eq } from 'drizzle-orm';
import { db } from '@/db';
import { messages } from '@/db/schema';

export const prerender = false;

export const POST: APIRoute = async ({ request, redirect }) => {
  const form = await request.formData();
  const action = String(form.get('_action') ?? '');
  const id = parseInt(String(form.get('id') ?? ''), 10);
  if (!Number.isFinite(id)) return redirect('/admin/messages');

  if (action === 'delete') {
    await db.delete(messages).where(eq(messages.id, id));
  } else if (action === 'toggle-read') {
    const [m] = await db.select().from(messages).where(eq(messages.id, id));
    if (m) await db.update(messages).set({ isRead: !m.isRead }).where(eq(messages.id, id));
  }
  return redirect('/admin/messages?saved=1');
};
