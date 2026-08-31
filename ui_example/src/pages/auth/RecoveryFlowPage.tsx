import { useEffect, useState } from "react";
import type { RecoveryFlow } from "@ory/client-fetch";
import { Recovery } from "@ory/elements-react/theme";
import {
  KRATOS_PUBLIC_URL,
  oryClient,
  oryConfiguration,
} from "@/foundation/lib/ory";
import { authElementComponents } from "./ory-components";

const recoveryEndpoint =
  `${KRATOS_PUBLIC_URL}/self-service/recovery/browser`;

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

export default function RecoveryFlowPage() {
  const [flow, setFlow] = useState<RecoveryFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const flowId = new URLSearchParams(window.location.search).get("flow");

  useEffect(() => {
    if (!flowId) {
      window.location.replace(recoveryEndpoint);
      return;
    }

    const loadFlow = async () => {
      try {
        setFlow(await oryClient.getRecoveryFlow({ id: flowId }));
      } catch (loadError) {
        if ([404, 410].includes(responseStatus(loadError) ?? 0)) {
          window.location.replace(recoveryEndpoint);
          return;
        }

        setError(
          loadError instanceof Error
            ? loadError.message
            : "Unable to load the recovery flow.",
        );
      }
    };

    void loadFlow();
  }, [flowId]);

  if (error) {
    return <p className="flow-status flow-status--error">{error}</p>;
  }

  if (!flow) {
    return <p className="flow-status">Loading account recovery…</p>;
  }

  return (
    <div className="auth-layout">
      <section className="auth-copy" aria-labelledby="recovery-copy-title">
        <div className="auth-copy__eyebrow">
          <span aria-hidden="true" />
          ACCOUNT RECOVERY
        </div>
        <h1 id="recovery-copy-title">Recover access, one verified step at a time.</h1>
        <p>
          Request a code for your registered email, then use it to open a
          privileged password settings flow.
        </p>
        <div className="auth-copy__note">
          <span className="auth-copy__note-icon" aria-hidden="true">
            ✓
          </span>
          <span>Development messages are captured safely by Mailpit.</span>
        </div>
      </section>
      <div className="auth-card">
        <Recovery
          flow={flow}
          config={oryConfiguration}
          components={authElementComponents}
        />
      </div>
    </div>
  );
}
