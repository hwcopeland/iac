import { sql } from 'drizzle-orm';
import type { LibSQLDatabase } from 'drizzle-orm/libsql';
import * as schema from './schema';
import { settings, teamMembers, testimonials, galleryImages } from './schema';

type DB = LibSQLDatabase<typeof schema>;

// Default content captured from the original valleygrowersgardencenter.com
// site. Only inserted when a table is empty, so editing in the admin never
// gets clobbered on the next boot.
export async function seed(db: DB) {
  const [{ count: settingsCount }] = await db
    .select({ count: sql<number>`count(*)` })
    .from(settings);

  if (settingsCount === 0) {
    await db.insert(settings).values({
      id: 1,
      businessName: 'Valley Growers Garden Center',
      tagline: 'Grown right here in Tennessee',
      phone: '615-890-9990',
      email: 'Susie.smsmdtn@gmail.com',
      address: '985 Middle Tennessee Blvd, Murfreesboro, TN 37130',
      facebookUrl: 'https://www.facebook.com/ValleyGrowersGardenCenter',
      hoursWeekday: '8:00 AM – 5:00 PM',
      hoursSaturday: '8:00 AM – 2:00 PM',
      hoursSunday: 'Closed',
      heroHeading: 'Welcome to Valley Growers',
      heroSubheading:
        'Plants, flowers, shrubs & trees grown in central Tennessee',
      aboutTitle: 'Who we are',
      aboutBody:
        'Valley Growers was established by siblings Bob Pile and Linda Washburn, who transformed a family farm in Fentress County into a major wholesale producer growing over two million plants a year. Leaving urban careers behind to pursue their agricultural vision, they have grown the business for nearly three decades.\n\nToday Valley Growers grows and sells plants, flowers, shrubs, and trees in central Tennessee — and our team is always happy to help you find exactly what your garden needs.',
      homeIntro:
        'For over 27 years, Valley Growers has served Murfreesboro with plants, flowers, and gardening supplies — backed by a knowledgeable, friendly staff happy to help you find exactly what your garden needs.',
    });
  }

  const [{ count: teamCount }] = await db
    .select({ count: sql<number>`count(*)` })
    .from(teamMembers);

  if (teamCount === 0) {
    await db.insert(teamMembers).values([
      {
        name: 'Dianna',
        role: 'B.S. Plant & Soil Science, MTSU',
        tenure: '13 years',
        favoritePlant: '',
        bio: 'Working at VG is like a home away from home; we are family and we all love what we do. Dianna loves helping new gardeners and rides horses on the family farm.',
        sortOrder: 1,
      },
      {
        name: 'Tara Neugebauer',
        role: 'B.S. Plant & Soil Science, MTSU (2015)',
        tenure: '13 years',
        favoritePlant: '',
        bio: 'I love my job and being able to help customers always makes me happy. Tara is a mother of four.',
        sortOrder: 2,
      },
      {
        name: 'Brooke Rose',
        role: '',
        tenure: '3 years',
        favoritePlant: 'Tulips',
        bio: "It's my happy place. Calming. I love seeing all the beautiful colors each season.",
        sortOrder: 3,
      },
      {
        name: 'Hampton',
        role: "Master's candidate in Chemistry, MTSU",
        tenure: '3 years',
        favoritePlant: 'Torch lily',
        bio: 'Hampton is pursuing a chemistry thesis at MTSU and values the community-oriented spirit at Valley Growers.',
        sortOrder: 4,
      },
      {
        name: 'Jerelyn',
        role: '',
        tenure: '3 years',
        favoritePlant: '',
        bio: 'I love that every day here is a little different. Jerelyn enjoys working with horses and gardening.',
        sortOrder: 5,
      },
      {
        name: 'Carol',
        role: 'B.A.S. Plant & Soil Science, MTSU',
        tenure: 'Since January 2025',
        favoritePlant: '',
        bio: 'A 16-year Army veteran, lifelong plant enthusiast, and CrossFit gym co-owner.',
        sortOrder: 6,
      },
      {
        name: 'Kenyon',
        role: 'MTSU senior — Plant & Soil Science, Entrepreneurship minor',
        tenure: 'Since February 2026',
        favoritePlant: '',
        bio: 'Runs his own landscaping business and is passionate about helping customers achieve their garden goals.',
        sortOrder: 7,
      },
      {
        name: 'Emily',
        role: 'Agribusiness major, MTSU',
        tenure: 'Since April',
        favoritePlant: '',
        bio: 'Loves learning about plants and sharing what she knows with customers.',
        sortOrder: 8,
      },
      {
        name: 'Kristy Thomas',
        role: '',
        tenure: '',
        favoritePlant: '',
        bio: 'Part of the Valley Growers family.',
        sortOrder: 9,
      },
    ]);
  }

  const [{ count: testimonialCount }] = await db
    .select({ count: sql<number>`count(*)` })
    .from(testimonials);

  if (testimonialCount === 0) {
    await db.insert(testimonials).values([
      {
        quote:
          'This is a great place to buy plants, flowers, herbs, vegetables, etc.',
        author: 'Moira A. Ragan',
        sortOrder: 1,
      },
      {
        quote:
          'They had a beautiful selection and everyone was wonderful to work with!!',
        author: 'Anna Smotherman',
        sortOrder: 2,
      },
      {
        quote: 'The friendliest and most helpful staff.',
        author: 'Nelson Vaught',
        sortOrder: 3,
      },
      {
        quote: 'Very knowledgeable employees. Always the best plants and flowers.',
        author: 'Sara Roy',
        sortOrder: 4,
      },
    ]);
  }

  const [{ count: galleryCount }] = await db
    .select({ count: sql<number>`count(*)` })
    .from(galleryImages);

  if (galleryCount === 0) {
    // Real photos pulled from the original site, served as static assets from
    // public/seed/. Plant/garden photos first, then the original gallery set.
    const files = [
      'home/IMG_2796.jpg', 'home/IMG_5895.jpeg', 'home/IMG_5896.jpeg',
      'home/IMG_5900.jpeg', 'home/IMG_5902.jpeg', 'home/IMG_5904.jpg',
      'home/IMG_2637.jpg', 'home/IMG_2638.jpg', 'home/IMG_2651.jpg',
      'gallery/IMG_2649.jpeg', 'gallery/IMG_2631.jpeg',
      'gallery/Screenshot_2026-04-11_at_12.38.36_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.38.56_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.39.20_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.39.42_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.43.02_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.43.10_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.43.32_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.48.40_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.50.09_AM.png',
      'gallery/Screenshot_2026-04-11_at_12.51.40_AM.png',
    ];
    await db.insert(galleryImages).values(
      files.map((f, i) => ({ src: `/seed/${f}`, caption: '', sortOrder: i + 1 })),
    );
  }
}
