// components/nav.tsx
import Link from 'next/link';

export default function Nav() {
  return (
    <nav className="bg-black">
      <div className="max-w-8xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16">
          <div className="flex items-center">
            <div className="flex-shrink-0 text-white font-bold">
                <Link href="/">
                    H4mp
                </Link>
            </div>
            <div className="hidden md:block">
              <div className="ml-10 flex items-baseline space-x-4">
                <Link href="/posts">
                    Posts
                </Link>
                <Link href="/about">
                    About
                </Link>
              </div>
            </div>
          </div>
        </div>
      </div>
    </nav>
  );
}
