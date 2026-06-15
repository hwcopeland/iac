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
        photoUrl: '/seed/team/IMG_1006.jpeg',
        photoPosition: '50% 18%',
        role: 'B.S. Plant & Soil Science, MTSU',
        tenure: '13 years',
        favoritePlant: '',
        bio: `Dianna has been with Valley Growers for 13 years and holds a bachelor's degree in Plant and Soil Science from MTSU. For her, working at VG is like a home away from home — the team is family, and they all love what they do. Her favorite part is helping new gardeners realize their dream of gardening. Outside of work, she enjoys riding horses on the family farm and growing her own food.`,
        sortOrder: 1,
      },
      {
        name: 'Emily',
        photoUrl: '/seed/team/IMG_5043_c.jpeg',
        photoPosition: '50% 30%',
        role: 'Agribusiness major, MTSU',
        tenure: 'Since April',
        favoritePlant: '',
        bio: `Emily is an Agribusiness major at MTSU and has been at Valley Growers since April. She has enjoyed learning more about each plant and watching everything bloom and grow, and she loves sharing her new knowledge with customers — and learning from them as well.`,
        sortOrder: 8,
      },
      {
        name: 'Brooke Rose',
        photoUrl: '/seed/team/IMG_5322.jpeg',
        photoPosition: '50% 30%',
        role: '',
        tenure: '3 years',
        favoritePlant: 'Tulips',
        bio: `Brooke has been at VG for 3 years, and her favorite flowers are tulips. What she loves about VG is interacting with customers and being around plants — it's her happy place, calming. She loves seeing all the beautiful colors each season.`,
        sortOrder: 3,
      },
      {
        name: 'Hampton',
        photoUrl: '/seed/team/IMG_5048.jpeg',
        photoPosition: '85% 32%',
        role: 'B.S. Chemistry, MTSU',
        tenure: '3 years',
        favoritePlant: 'Torch lily',
        bio: `Hampton has been part of the team at Valley Growers for three years, where the community-oriented spirit of the small business is what makes the work most rewarding. A favorite among the many plants is the torch lily, with its bold, fiery blooms. Outside of Valley Growers, Hampton studies chemistry at MTSU.`,
        sortOrder: 4,
      },
      {
        name: 'Kenyon',
        photoUrl: '/seed/team/IMG_1217_c.jpeg',
        photoPosition: '50% 20%',
        role: 'MTSU senior — Plant & Soil Science, Entrepreneurship minor',
        tenure: 'Since February 2026',
        favoritePlant: '',
        bio: `Kenyon is a senior at MTSU majoring in Plant and Soil Science with a minor in entrepreneurship, and he graduated high school with an associate's degree in applied sciences. He runs his own landscaping business and has worked at Valley Growers since February 2026. Over the last few months he's grown a lot in his overall knowledge of plants, and he hopes to keep improving as he heads into his senior year. He loves working with his coworkers and helping customers accomplish their desired results for their gardens.`,
        sortOrder: 7,
      },
      {
        name: 'Kristy Thomas',
        photoUrl: '/seed/team/IMG_5035_c.jpeg',
        photoPosition: '50% 25%',
        role: "Valley Growers' best water girl eva!",
        tenure: '',
        favoritePlant: '',
        bio: `Kristy has been married for 25 years and is a mom of two grown girls. A CrossFitter for 15 years, she's co-owned a CrossFit gym in Christiana since 2020. Now a retired full-time mom, she's Valley Growers' part-time water girl — she loves working here because the environment is so friendly and beautiful, enjoys chatting with the customers, and says she has the best coworkers to work with.`,
        sortOrder: 9,
      },
      {
        name: 'Tara Neugebauer',
        photoUrl: '/seed/team/1000006145.JPG',
        photoPosition: '50% 15%',
        role: 'B.S. Plant & Soil Science, MTSU (2015)',
        tenure: '13 years',
        favoritePlant: '',
        bio: `Tara has been with Valley Growers for 13 years and holds a Bachelor's in Plant and Soil Science from MTSU (2015). She loves her job, and being able to help customers always makes her happy. She loves all flowers, so choosing a favorite is hard. She's a devoted mom to four wonderful children.`,
        sortOrder: 2,
      },
      {
        name: 'Jerelyn',
        photoUrl: '/seed/team/IMG_0994_c.jpeg',
        photoPosition: '50% 25%',
        role: '',
        tenure: '3 years',
        favoritePlant: '',
        bio: `Jerelyn has been part of the team for 3 years. Nature and outdoor work have always been a big part of her life, so working at Valley Growers feels like a natural fit. She's always enjoyed environments that are welcoming, creative, and hands-on, and she loves that every day here is a little different — whether she's helping care for plants or helping customers find the perfect flowers, greenery, or seasonal inspiration. Outside of work, she enjoys spending time with horses and the slower, peaceful side of being outdoors, and playing in her garden.`,
        sortOrder: 5,
      },
      {
        name: 'Carol',
        photoUrl: '/seed/team/IMG_1211.jpeg',
        photoPosition: '50% 14%',
        role: 'B.A.S. Plant & Soil Science, MTSU',
        tenure: 'Since January 2025',
        favoritePlant: '',
        bio: `Carol was medically retired after 16 years in the Army and started working at Valley Growers in January 2025 while attending MTSU, where she recently earned a Bachelor's of Applied Science in Plant & Soil Science. She's been working with and learning about plants for nearly 10 years — at one point she had over 100 houseplants! She loves working at Valley Growers and sharing her knowledge and experience with the community.`,
        sortOrder: 6,
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
