// app/posts/layout.tsx
import { ReactNode } from 'react';
import Link from 'next/link';
import Nav from '@/components/Nav';
import { getAllPosts } from '@/lib/posts';
import HexBackground from '@/components/HexBackground';

export default async function PostsLayout({ children }: { children: ReactNode }) {
  const posts = await getAllPosts();
  
  return (
    <>
      <HexBackground />
      <Nav />
      <div className="flex min-h-screen p-4 gap-4">
        {/* Sidebar */}
        <aside className="w-1/4 p-4 border-r rounded-lg bg-black/40 shadow-md">
          <h2 className="text-xl font-bold mb-4">Blog Posts</h2>
          <ul>
          {posts.map((post) => (
            <li key={post.slug} className="mb-2">
              <Link
                href={`/posts/${post.slug}`}
                className="block rounded transition-all duration-300 bg-gray mx-4 hover:bg-black/80 hover:mr-0"
              >
                <div className="px-4 py-2">
                  {post.title}
                </div>
              </Link>
            </li>
          ))}
        </ul>
        </aside>
        {/* Main Content */}
        <main
          className="relative z-10 flex-1 p-4 rounded-lg bg-black/30"
          style={{
            boxShadow: "inset 0 0 10px 8px rgba(0,0,0,0.7), inset 0 0 20px 12px rgba(22, 22, 22, 0)"
          }}
        >
          {children}
        </main>
      </div>
    </>
  );
}
