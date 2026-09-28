export type ApiToken = {
  id: string;
  name: string;
  scopes: string[];
  status: string;
  expire_time: string;
  created_at: string;
};

export type ApiTokenResult = {
  token: ApiToken;
  secret?: string;
};

export class ApiTokenError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = "ApiTokenError";
    this.status = status;
    this.body = body;
  }
}

function errorMessage(body: unknown, status: number): string {
  if (typeof body === "object" && body !== null && "error" in body) {
    const error = (body as { error?: unknown }).error;
    if (typeof error === "string") return error;
  }
  return `Request failed with HTTP ${status}`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  const text = await response.text();
  const body: unknown = text ? JSON.parse(text) : {};
  if (!response.ok) throw new ApiTokenError(errorMessage(body, response.status), response.status, body);
  return body as T;
}

export function listApiTokens(): Promise<{ tokens: ApiToken[] }> {
  return request("/v1/auth/tokens");
}

export function createApiToken(input: { name: string; scopes: string[]; ttl: string }): Promise<ApiTokenResult> {
  return request("/v1/auth/tokens", { method: "POST", body: JSON.stringify(input) });
}

export function revokeApiToken(id: string): Promise<{ id: string; status: string }> {
  return request(`/v1/auth/tokens/${encodeURIComponent(id)}/revoke`, { method: "POST", body: JSON.stringify({}) });
}

export function rotateApiToken(id: string): Promise<ApiTokenResult> {
  return request(`/v1/auth/tokens/${encodeURIComponent(id)}/rotate`, { method: "POST", body: JSON.stringify({}) });
}
