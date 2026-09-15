import type {
  ListOrganizationsResponse,
  SearchContentsResponse,
  SocialRequestResult,
} from "./types";

export class SocialApiError extends Error {
  readonly status: number;
  readonly body: unknown;
  readonly path: string;

  constructor(message: string, status: number, body: unknown, path: string) {
    super(message);
    this.name = "SocialApiError";
    this.status = status;
    this.body = body;
    this.path = path;
  }
}

function message(body: unknown, status: number): string {
  if (typeof body === "object" && body !== null) {
    const value = body as { error?: string | { message?: string }; message?: string };
    if (typeof value.error === "string") return value.error;
    if (typeof value.error?.message === "string") return value.error.message;
    if (typeof value.message === "string") return value.message;
  }
  return `Request failed with HTTP ${status}`;
}

async function request<T>(path: string, headers?: Record<string, string>): Promise<SocialRequestResult<T>> {
  const response = await fetch(path, { credentials: "include", headers });
  const text = await response.text();
  let data: unknown = {};
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  if (!response.ok) {
    throw new SocialApiError(message(data, response.status), response.status, data, path);
  }
  return { data: data as T, method: "GET", path, status: response.status };
}

export function listMyOrganizations(): Promise<SocialRequestResult<ListOrganizationsResponse>> {
  return request("/v1/social/me/organizations");
}

export function searchContents(
  organizationId: string,
  keyword: string,
  faultMode: "" | "unavailable" | "delay" = "",
): Promise<SocialRequestResult<SearchContentsResponse>> {
  const query = new URLSearchParams({ keyword });
  const headers = faultMode ? { "x-ddd-fault-mode": faultMode } : undefined;
  return request(
    `/v1/social/organizations/${encodeURIComponent(organizationId)}/contents?${query.toString()}`,
    headers,
  );
}
