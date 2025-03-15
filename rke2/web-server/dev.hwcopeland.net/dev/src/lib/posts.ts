// lib/posts.ts
export async function getAllPosts() {
    // Replace with your actual logic to fetch a list of posts
    return [
      { slug: 'hello-world', title: 'Hello World' },
      { slug: 'next-mdx', title: 'Using MDX with Next.js 13' },
    ];
  }
  
  export async function getPostBySlug(slug: string) {
    // Replace with your actual logic to fetch post content.
    // Here we simply return a dummy MDX string.
    const posts = {
      'hello-world': { title: 'Hello World', content: '# Hello, Supreme Leader!\n\nThis is your first blog post.' },
      'next-mdx': { title: 'Using MDX with Next.js 13', content: '# Next.js 13 & MDX\n\nRender MDX with the new App Router.' },
    };
    return posts[slug] || null;
  }
  