import type {
  ListCrawlContentsResponse,
  ListMyOrganizationsResponse,
  StartCrawlTaskResponse,
  UpdateKeywordsResponse,
  XhsRequestResult,
} from "@/domains/xhs/types";

export class XhsApiError extends Error {
  readonly status: number;
  readonly body: unknown;
  readonly method: "GET" | "POST" | "PUT";
  readonly path: string;

  constructor(
    message: string,
    status: number,
    body: unknown,
    method: "GET" | "POST" | "PUT",
    path: string,
  ) {
    super(message);
    this.name = "XhsApiError";
    this.status = status;
    this.body = body;
    this.method = method;
    this.path = path;
  }
}

function endpoint(organizationId: string, resource: string): string {
  return `/v1/xhs/organizations/${encodeURIComponent(organizationId)}/crawl/${resource}`;
}

function errorMessage(body: unknown, status: number): string {
  if (typeof body === "object" && body !== null) {
    const value = body as {
      error?: string | { message?: string };
      message?: string;
    };
    if (typeof value.error === "string") return value.error;
    if (typeof value.error?.message === "string") return value.error.message;
    if (typeof value.message === "string") return value.message;
  }
  return `Request failed with HTTP ${status}`;
}

async function request<T>(
  method: XhsRequestResult<T>["method"],
  path: string,
  body?: unknown,
): Promise<XhsRequestResult<T>> {
  const response = await fetch(path, {
    method,
    credentials: "include",
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
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
    throw new XhsApiError(
      errorMessage(data, response.status),
      response.status,
      data,
      method,
      path,
    );
  }

  return { data: data as T, method, path, status: response.status };
}

export function startCrawlTask(
  organizationId: string,
  keywords: string[],
): Promise<XhsRequestResult<StartCrawlTaskResponse>> {
  return request("POST", endpoint(organizationId, "tasks"), { keywords });
}

export function listMyOrganizations(): Promise<
  XhsRequestResult<ListMyOrganizationsResponse>
> {
  return request("GET", "/v1/xhs/me/organizations");
}

export function listCrawlContents(
  organizationId: string,
): Promise<XhsRequestResult<ListCrawlContentsResponse>> {
  return request("GET", endpoint(organizationId, "contents"));
}

export function updateKeywords(
  organizationId: string,
  values: string[],
): Promise<XhsRequestResult<UpdateKeywordsResponse>> {
  return request("PUT", endpoint(organizationId, "keywords"), { values });
}
