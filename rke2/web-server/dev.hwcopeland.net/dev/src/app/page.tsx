import type { AppProps } from 'next/app';
import Nav from '@/components/Nav.tsx';
import './globals.css'; // Tailwind CSS should be included here
import HexBackground from '@/components/HexBackground.tsx';

export default function MyApp({ Component, pageProps }: AppProps) {
  return (
    <>
      <HexBackground />
      <Nav />
    </>
  );
}
