// app/posts/[slug]/page.tsx
import { getPostBySlug, getAllPosts } from '@/lib/posts';
import { notFound } from 'next/navigation';
import ClientMDXRemote from '@/components/ClientMDXRemote';

type PostPageProps = {
  params: { slug: string };
};

export async function generateStaticParams() {
  const posts = await getAllPosts();
  return posts.map((post) => ({ slug: post.slug }));
}

export default async function PostPage({ params }: PostPageProps) {
  const post = await getPostBySlug(params.slug);
  if (!post) {
    notFound();
  }
  
  // Assume post.content is the MDX string that you serialized earlier.
  // You should pre-serialize your MDX content in your data fetching method.
  const { serialize } = await import('next-mdx-remote/serialize');
  const source = await serialize(post.content);
  
  return (
    <article>
      <h1 className="text-3xl font-bold mb-4">{post.title}</h1>
      <ClientMDXRemote source={source} />
    </article>
  );
}
