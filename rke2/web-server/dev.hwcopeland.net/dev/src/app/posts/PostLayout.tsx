// app/posts/layout.tsx
import { ReactNode } from 'react';
import Link from 'next/link';
import { getAllPosts } from '@/lib/posts'; // Your data fetching helper

export default async function PostsLayout({ children }: { children: ReactNode }) {
  // Fetch all posts (each post should have at least slug and title)
  const posts = await getAllPosts();
  return (
    <div className="flex min-h-screen">
      {/* Sidebar */}
      <aside className="w-1/4 p-4 border-r">
        <h2 className="text-xl font-bold mb-4">Blog Posts</h2>
        <ul>
          {posts.map((post) => (
            <li key={post.slug} className="mb-2">
              <Link href={`/posts/${post.slug}`}>
                {post.title}
              </Link>
            </li>
          ))}
        </ul>
      </aside>
      {/* Main Content */}
      <main className="flex-1 p-4">
        {children}
      </main>
    </div>
  );
}
