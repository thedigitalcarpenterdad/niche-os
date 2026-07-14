export class APIError extends Error {
  constructor(
    public status: number,
    body: string,
  ) {
    super(body);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const method = (init.method ?? "GET").toUpperCase();
  headers.set("Accept", "application/json");
  if (init.body && !(init.body instanceof FormData))
    headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD", "OPTIONS", "TRACE"].includes(method)) headers.set("X-ClickClack-CSRF", "1");
  const response = await fetch(path, { ...init, headers, credentials: "include" });
  if (!response.ok) {
    throw new APIError(response.status, await response.text());
  }
  return response.json() as Promise<T>;
}

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  const method = (init.method ?? "GET").toUpperCase();
  if (!["GET", "HEAD", "OPTIONS", "TRACE"].includes(method)) headers.set("X-ClickClack-CSRF", "1");
  return fetch(path, { ...init, headers, credentials: "include" });
}
