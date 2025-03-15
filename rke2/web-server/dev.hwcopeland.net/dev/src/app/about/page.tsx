import type { AppProps } from 'next/app';
import Nav from '@/components/Nav.tsx';
import '../globals.css';
import MDXDocument from '@/components/MDXDocument';

export default function MyApp({ Component, pageProps }: AppProps) {
  return (
    <>
      <Nav />
      <MDXDocument {...pageProps} />
    </>
  );
}