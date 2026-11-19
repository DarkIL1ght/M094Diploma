export class ApiError extends Error {
  constructor(message, { kind, status, code, details, retryAfter, cause }) {
    super(message, { cause });
    this.name = 'ApiError';
    this.kind = kind;
    this.status = status;
    this.code = code;
    this.details = details;
    this.retryAfter = retryAfter;
  }
}

export function createApi({ baseUrl = '', credentials = 'same-origin' } = {}) {
  const base = baseUrl.replace(/\/+$/, '');

  async function request(path, { method = 'GET', json, headers, signal } = {}) {
    if (!path.startsWith('/api/')) throw new TypeError('Ожидается путь из контракта /api/.');
    const requestHeaders = new globalThis.Headers(headers);
    requestHeaders.set('Accept', 'application/json');
    if (json !== undefined) requestHeaders.set('Content-Type', 'application/json');
    const body = json === undefined ? undefined : JSON.stringify(json);

    let response;
    let data;
    try {
      response = await fetch(`${base}${path}`, {
        method,
        headers: requestHeaders,
        credentials,
        body,
        signal,
      });
      if (response.status === 204) return undefined;
      const responseText = await response.text();
      try {
        data = JSON.parse(responseText);
      } catch (cause) {
        if (response.ok) {
          throw new ApiError('Сервер вернул некорректный JSON.', {
            kind: 'response', status: response.status, cause,
          });
        }
      }
    } catch (cause) {
      if (signal?.aborted || cause?.name === 'AbortError' || cause instanceof ApiError) throw cause;
      throw new ApiError('Не удалось связаться с сервером.', { kind: 'network', cause });
    }

    if (!response.ok) {
      const retryAfter = response.headers.get('Retry-After');
      throw new ApiError(typeof data?.message === 'string' ? data.message : `Ошибка HTTP ${response.status}.`, {
        kind: 'http',
        status: response.status,
        code: data?.code,
        details: data?.details,
        retryAfter: retryAfter !== null && /^\d+$/.test(retryAfter) ? Number(retryAfter) : undefined,
      });
    }

    return data;
  }

  return {
    request,
    getMe: (options) => request('/api/me', options),
    listSubjects: (options) => request('/api/subjects', options),
    listTopics: (subject, options) => request(`/api/subjects/${encodeURIComponent(subject)}/topics`, options),
  };
}
