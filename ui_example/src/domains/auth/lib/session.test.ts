import type { Session } from "@ory/client-fetch";
import { describe, expect, it } from "vitest";
import { getIdentitySummary } from "./session";

function sessionWithTraits(traits: unknown): Session {
  return {
    id: "session-id",
    identity: {
      id: "identity-id",
      schema_id: "default",
      schema_url: "http://kratos/schemas/default",
      traits,
    },
  };
}

describe("getIdentitySummary", () => {
  it("prefers the person's name while preserving contact traits", () => {
    expect(
      getIdentitySummary(
        sessionWithTraits({
          email: "person@example.com",
          phone: "+8613800000000",
          name: { first: "Tao", last: "Li" },
        }),
      ),
    ).toEqual({
      displayName: "Tao Li",
      email: "person@example.com",
      phone: "+8613800000000",
    });
  });

  it("falls back to the identity id when traits are empty", () => {
    expect(getIdentitySummary(sessionWithTraits({}))).toEqual({
      displayName: "identity-id",
      email: null,
      phone: null,
    });
  });
});
