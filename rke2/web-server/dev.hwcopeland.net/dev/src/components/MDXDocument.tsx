// components/MDXDocument.tsx
import { MDXRemote } from 'next-mdx-remote';
import type { MDXRemoteSerializeResult } from 'next-mdx-remote';

type MDXDocumentProps = {
  source: MDXRemoteSerializeResult;
};

export default function MDXDocument({ source }: MDXDocumentProps) {
  return (
    <div className="prose mx-auto p-4">
      <MDXRemote {...source} />
    </div>
  );
}