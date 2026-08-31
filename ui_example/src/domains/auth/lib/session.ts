import type { Session } from "@ory/client-fetch";

type IdentityTraits = {
  email?: unknown;
  phone?: unknown;
  name?: {
    first?: unknown;
    last?: unknown;
  };
};

export type IdentitySummary = {
  displayName: string;
  email: string | null;
  phone: string | null;
};

function nonEmptyString(value: unknown): string | null {
  return typeof value === "string" && value.trim() ? value.trim() : null;
}

export function getIdentitySummary(session: Session): IdentitySummary {
  const traits = (session.identity?.traits ?? {}) as IdentityTraits;
  const email = nonEmptyString(traits.email);
  const phone = nonEmptyString(traits.phone);
  const firstName = nonEmptyString(traits.name?.first);
  const lastName = nonEmptyString(traits.name?.last);
  const fullName = [firstName, lastName].filter(Boolean).join(" ");

  return {
    displayName:
      fullName || email || phone || session.identity?.id || "authenticated user",
    email,
    phone,
  };
}
