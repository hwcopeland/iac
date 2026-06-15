import { createClient } from '@libsql/client';
import { drizzle } from 'drizzle-orm/libsql';
import { mkdirSync } from 'node:fs';
import { join, isAbsolute } from 'node:path';
import * as schema from './schema';
import { seed } from './seed';

// Everything that must survive restarts lives under DATA_DIR: the SQLite file
// and the uploaded images. In the cluster this is a mounted PVC; locally it is
// ./data. We create it eagerly so a fresh checkout / fresh volume just works.
const DATA_DIR = process.env.DATA_DIR ?? './data';
export const UPLOAD_DIR = join(DATA_DIR, 'uploads');
const DB_PATH = join(DATA_DIR, 'vg.db');

mkdirSync(UPLOAD_DIR, { recursive: true });

const client = createClient({
  url: `file:${isAbsolute(DB_PATH) ? DB_PATH : './' + DB_PATH}`,
});

export const db = drizzle(client, { schema });

// Create the schema if it does not exist, then seed default content on an
// empty database. Idempotent: safe to run on every boot.
async function initDb() {
  await client.batch(
    [
      `CREATE TABLE IF NOT EXISTS settings (
        id INTEGER PRIMARY KEY,
        business_name TEXT NOT NULL,
        tagline TEXT NOT NULL,
        phone TEXT NOT NULL,
        email TEXT NOT NULL,
        address TEXT NOT NULL,
        facebook_url TEXT NOT NULL,
        hours_weekday TEXT NOT NULL,
        hours_saturday TEXT NOT NULL,
        hours_sunday TEXT NOT NULL,
        hero_heading TEXT NOT NULL,
        hero_subheading TEXT NOT NULL,
        about_title TEXT NOT NULL,
        about_body TEXT NOT NULL,
        home_intro TEXT NOT NULL
      )`,
      `CREATE TABLE IF NOT EXISTS team_members (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        role TEXT NOT NULL DEFAULT '',
        tenure TEXT NOT NULL DEFAULT '',
        favorite_plant TEXT NOT NULL DEFAULT '',
        bio TEXT NOT NULL DEFAULT '',
        photo_url TEXT NOT NULL DEFAULT '',
        sort_order INTEGER NOT NULL DEFAULT 0
      )`,
      `CREATE TABLE IF NOT EXISTS gallery_images (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        src TEXT NOT NULL,
        caption TEXT NOT NULL DEFAULT '',
        sort_order INTEGER NOT NULL DEFAULT 0
      )`,
      `CREATE TABLE IF NOT EXISTS testimonials (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        quote TEXT NOT NULL,
        author TEXT NOT NULL DEFAULT '',
        sort_order INTEGER NOT NULL DEFAULT 0
      )`,
      `CREATE TABLE IF NOT EXISTS messages (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        email TEXT NOT NULL,
        body TEXT NOT NULL,
        is_read INTEGER NOT NULL DEFAULT 0,
        created_at INTEGER NOT NULL
      )`,
    ],
    'write',
  );

  await seed(db);
}

await initDb();

export { schema };
