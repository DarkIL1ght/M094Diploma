import { notFound, routes } from './routes.js';

const segments = (path) => path === '/' ? [] : path.slice(1).replace(/\/$/, '').split('/');

const patterns = routes.map((route) => ({ route, parts: segments(route.path) }));

export function matchRoute(pathname) {
  try {
    const parts = segments(pathname).map(decodeURIComponent);

    for (const { route, parts: pattern } of patterns) {
      if (pattern.length !== parts.length) continue;

      const params = {};
      const matches = pattern.every((part, index) => {
        if (!part.startsWith(':')) return part === parts[index];
        if (!parts[index]) return false;
        params[part.slice(1)] = parts[index];
        return true;
      });
      if (matches) return { route, params };
    }
  } catch (error) {
    if (!(error instanceof URIError)) throw error;
  }

  return { route: notFound, params: {} };
}

export function startRouter(render) {
  const showCurrentPage = () => render(matchRoute(window.location.pathname));

  document.addEventListener('click', (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey ||
        event.ctrlKey || event.shiftKey || event.altKey) return;

    const link = event.target.closest?.('a[data-route]');
    if (!link || link.target || link.hasAttribute('download')) return;

    const url = new URL(link.href);
    if (url.origin !== window.location.origin) return;

    event.preventDefault();
    if (url.href !== window.location.href) {
      window.history.pushState(null, '', url);
    }
    showCurrentPage();
  });

  window.addEventListener('popstate', showCurrentPage);
  render(matchRoute(window.location.pathname), { focus: false });
}
