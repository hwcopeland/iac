import { sqliteTable, integer, text } from 'drizzle-orm/sqlite-core';

// Singleton row (id = 1) holding all the editable site-wide content: contact
// details, hours, and the hero / about copy shown on the public pages.
export const settings = sqliteTable('settings', {
  id: integer('id').primaryKey(),
  businessName: text('business_name').notNull(),
  tagline: text('tagline').notNull(),
  phone: text('phone').notNull(),
  email: text('email').notNull(),
  address: text('address').notNull(),
  facebookUrl: text('facebook_url').notNull(),
  hoursWeekday: text('hours_weekday').notNull(),
  hoursSaturday: text('hours_saturday').notNull(),
  hoursSunday: text('hours_sunday').notNull(),
  heroHeading: text('hero_heading').notNull(),
  heroSubheading: text('hero_subheading').notNull(),
  aboutTitle: text('about_title').notNull(),
  aboutBody: text('about_body').notNull(),
  homeIntro: text('home_intro').notNull(),
});

export const teamMembers = sqliteTable('team_members', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  name: text('name').notNull(),
  role: text('role').notNull().default(''),
  tenure: text('tenure').notNull().default(''),
  favoritePlant: text('favorite_plant').notNull().default(''),
  bio: text('bio').notNull().default(''),
  photoUrl: text('photo_url').notNull().default(''),
  photoPosition: text('photo_position').notNull().default('50% 20%'),
  sortOrder: integer('sort_order').notNull().default(0),
});

export const galleryImages = sqliteTable('gallery_images', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  src: text('src').notNull(),
  caption: text('caption').notNull().default(''),
  sortOrder: integer('sort_order').notNull().default(0),
});

export const testimonials = sqliteTable('testimonials', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  quote: text('quote').notNull(),
  author: text('author').notNull().default(''),
  sortOrder: integer('sort_order').notNull().default(0),
});

// Contact-form submissions land here so they show up in the admin inbox.
export const messages = sqliteTable('messages', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  name: text('name').notNull(),
  email: text('email').notNull(),
  body: text('body').notNull(),
  isRead: integer('is_read', { mode: 'boolean' }).notNull().default(false),
  createdAt: integer('created_at', { mode: 'timestamp' }).notNull(),
});

export type Settings = typeof settings.$inferSelect;
export type TeamMember = typeof teamMembers.$inferSelect;
export type GalleryImage = typeof galleryImages.$inferSelect;
export type Testimonial = typeof testimonials.$inferSelect;
export type Message = typeof messages.$inferSelect;
