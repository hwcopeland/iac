import { asc, desc, eq } from 'drizzle-orm';
import { db } from '@/db';
import {
  settings,
  teamMembers,
  galleryImages,
  testimonials,
  messages,
  type Settings,
} from '@/db/schema';

export async function getSettings(): Promise<Settings> {
  const [row] = await db.select().from(settings).where(eq(settings.id, 1));
  return row;
}

export function getTeam() {
  return db.select().from(teamMembers).orderBy(asc(teamMembers.sortOrder), asc(teamMembers.id));
}

export function getGallery() {
  return db
    .select()
    .from(galleryImages)
    .orderBy(asc(galleryImages.sortOrder), asc(galleryImages.id));
}

export function getTestimonials() {
  return db
    .select()
    .from(testimonials)
    .orderBy(asc(testimonials.sortOrder), asc(testimonials.id));
}

export function getMessages() {
  return db.select().from(messages).orderBy(desc(messages.createdAt));
}
