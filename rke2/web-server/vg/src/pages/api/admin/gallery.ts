import type { APIRoute } from 'astro';
import { eq } from 'drizzle-orm';
import { db } from '@/db';
import { galleryImages } from '@/db/schema';
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
      await db.delete(galleryImages).where(eq(galleryImages.id, id));
      return redirect('/admin/gallery?saved=1');
    }

    if (action === 'update') {
      await db
        .update(galleryImages)
        .set({
          caption: String(form.get('caption') ?? '').trim(),
          sortOrder: num(form.get('sortOrder')),
        })
        .where(eq(galleryImages.id, id));
      return redirect('/admin/gallery?saved=1');
    }

    // create: either an uploaded file or a pasted image URL
    const uploaded = await saveUpload(form.get('image'));
    const src = uploaded ?? String(form.get('src') ?? '').trim();
    if (!src) return redirect('/admin/gallery?err=Upload+a+file+or+enter+an+image+URL');

    await db.insert(galleryImages).values({
      src,
      caption: String(form.get('caption') ?? '').trim(),
      sortOrder: num(form.get('sortOrder')),
    });
    return redirect('/admin/gallery?saved=1');
  } catch (e) {
    return redirect(`/admin/gallery?err=${encodeURIComponent((e as Error).message)}`);
  }
};
