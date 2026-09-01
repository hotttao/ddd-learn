import { afterEach, describe, expect, it, vi } from "vitest";
import { listCrawlContents, startCrawlTask } from "./api";

afterEach(() => vi.restoreAllMocks());

describe("xhs api", () => {
  it("uses the service-owned prefix and includes the browser session", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response('{"task_id":"task-1","status":"pending"}', {
          status: 200,
        }),
      );

    await startCrawlTask("team/a", ["Ory"]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/xhs/organizations/team%2Fa/crawl/tasks",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: '{"keywords":["Ory"]}',
      }),
    );
  });

  it("preserves the gateway status and error body", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response('{"error":{"message":"not authenticated"}}', {
        status: 401,
      }),
    );

    const request = listCrawlContents("demo");

    await expect(request).rejects.toMatchObject({
      status: 401,
      message: "not authenticated",
      body: { error: { message: "not authenticated" } },
      method: "GET",
      path: "/v1/xhs/organizations/demo/crawl/contents",
    });
  });
});
