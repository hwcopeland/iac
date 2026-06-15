import type { APIRoute } from 'astro';
import { db } from '@/db';
import { messages } from '@/db/schema';

export const prerender = false;

export const POST: APIRoute = async ({ request, redirect }) => {
  try {
    const form = await request.formData();
    const name = String(form.get('name') ?? '').trim();
    const email = String(form.get('email') ?? '').trim();
    const body = String(form.get('body') ?? '').trim();

    if (!name || !email || !body) return redirect('/contact?error=1');

    await db.insert(messages).values({ name, email, body, createdAt: new Date() });
    return redirect('/contact?sent=1');
  } catch {
    return redirect('/contact?error=1');
  }
};
