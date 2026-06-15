import { defineMiddleware } from 'astro:middleware';
import { SESSION_COOKIE, isValidSession } from '@/lib/auth';

// Anything under /admin or /api/admin requires a valid admin session. The login
// page and the public endpoints (login, contact form) stay open.
export const onRequest = defineMiddleware(async (context, next) => {
  const { pathname } = context.url;
  const isProtected =
    (pathname.startsWith('/admin') && pathname !== '/admin/login') ||
    pathname.startsWith('/api/admin');

  if (isProtected) {
    const token = context.cookies.get(SESSION_COOKIE)?.value;
    if (!(await isValidSession(token))) {
      if (pathname.startsWith('/api/')) {
        return new Response('Unauthorized', { status: 401 });
      }
      return context.redirect('/admin/login');
    }
  }

  return next();
});
