import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { FrontendApi, Session } from "@ory/client-fetch";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OrySessionProvider, useOrySession } from "./ory-session";

function SessionProbe() {
  const { status, session, logout } = useOrySession();

  return (
    <div>
      <span>{status}</span>
      <span>{session?.id}</span>
      <button type="button" onClick={() => void logout()}>
        Sign out
      </button>
    </div>
  );
}

function createClient() {
  return {
    toSession: vi.fn<FrontendApi["toSession"]>(),
    createBrowserLogoutFlow:
      vi.fn<FrontendApi["createBrowserLogoutFlow"]>(),
  };
}

const session: Session = {
  id: "session-id",
  active: true,
};

describe("OrySessionProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("exposes the active browser session and starts browser logout", async () => {
    const client = createClient();
    client.toSession.mockResolvedValue(session);
    client.createBrowserLogoutFlow.mockResolvedValue({
      logout_token: "logout-token",
      logout_url: "http://kratos/self-service/logout?token=logout-token",
    });

    render(
      <OrySessionProvider client={client}>
        <SessionProbe />
      </OrySessionProvider>,
    );

    expect(await screen.findByText("authenticated")).toBeTruthy();
    expect(screen.getByText("session-id")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => {
      expect(window.location.assign).toHaveBeenCalledWith(
        "http://kratos/self-service/logout?token=logout-token",
      );
    });
  });

  it("treats a 401 response as an anonymous browser", async () => {
    const client = createClient();
    client.toSession.mockRejectedValue({ response: { status: 401 } });

    render(
      <OrySessionProvider client={client}>
        <SessionProbe />
      </OrySessionProvider>,
    );

    expect(await screen.findByText("anonymous")).toBeTruthy();
  });
});
