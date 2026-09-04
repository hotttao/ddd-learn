import type { VerificationFlow } from "@ory/client-fetch";
import { Verification } from "@ory/elements-react/theme";
import { useEffect, useState } from "react";
import {
  KRATOS_PUBLIC_URL,
  oryClient,
  oryConfiguration,
} from "@/foundation/lib/ory";
import { authElementComponents } from "./ory-components";

const verificationEndpoint = `${KRATOS_PUBLIC_URL}/self-service/verification/browser`;

function responseStatus(error: unknown): number | null {
  if (
    typeof error !== "object" ||
    error === null ||
    !("response" in error) ||
    typeof error.response !== "object" ||
    error.response === null ||
    !("status" in error.response) ||
    typeof error.response.status !== "number"
  ) {
    return null;
  }

  return error.response.status;
}

export default function VerificationFlowPage() {
  const [flow, setFlow] = useState<VerificationFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const flowId = new URLSearchParams(window.location.search).get("flow");

  useEffect(() => {
    if (!flowId) {
      window.location.replace(verificationEndpoint);
      return;
    }

    const loadFlow = async () => {
      try {
        setFlow(await oryClient.getVerificationFlow({ id: flowId }));
      } catch (loadError) {
        if ([404, 410].includes(responseStatus(loadError) ?? 0)) {
          window.location.replace(verificationEndpoint);
          return;
        }

        setError(
          loadError instanceof Error
            ? loadError.message
            : "Unable to load the email verification flow.",
        );
      }
    };

    void loadFlow();
  }, [flowId]);

  if (error) {
    return <p className="flow-status flow-status--error">{error}</p>;
  }

  if (!flow) {
    return <p className="flow-status">Loading email verification…</p>;
  }

  return (
    <div className="auth-layout">
      <section className="auth-copy" aria-labelledby="verification-copy-title">
        <div className="auth-copy__eyebrow">
          <span aria-hidden="true" />
          VERIFY YOUR EMAIL
        </div>
        <h1 id="verification-copy-title">Confirm where we can reach you.</h1>
        <p>
          Enter the verification code from your email before signing in to
          protected services.
        </p>
        <div className="auth-copy__note">
          <span className="auth-copy__note-icon" aria-hidden="true">
            ✓
          </span>
          <span>Development messages are captured safely by Mailpit.</span>
        </div>
      </section>
      <div className="auth-card">
        <Verification
          flow={flow}
          config={oryConfiguration}
          components={authElementComponents}
        />
      </div>
    </div>
  );
}
