// app/posts/page.tsx
import Link from 'next/link';
import { getAllPosts } from '@/lib/posts';

export default async function PostsIndex() {
  const posts = await getAllPosts();
  return (
    <div>
      <h1 className="text-3xl font-bold mb-4">All Posts</h1>
      <ul>
        {posts.map((post) => (
          <li key={post.slug} className="mb-2">
            <Link href={`/posts/${post.slug}`}>{post.title}</Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
