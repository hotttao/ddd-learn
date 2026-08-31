import { useEffect, useState } from "react";
import type { SettingsFlow } from "@ory/client-fetch";
import { Settings } from "@ory/elements-react/theme";
import {
  KRATOS_PUBLIC_URL,
  oryClient,
  oryConfiguration,
} from "@/foundation/lib/ory";
import { authElementComponents } from "./ory-components";

const settingsEndpoint =
  `${KRATOS_PUBLIC_URL}/self-service/settings/browser`;

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

export default function AccountSettingsPage() {
  const [flow, setFlow] = useState<SettingsFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const flowId = new URLSearchParams(window.location.search).get("flow");

  useEffect(() => {
    if (!flowId) {
      window.location.replace(settingsEndpoint);
      return;
    }

    const loadFlow = async () => {
      try {
        setFlow(await oryClient.getSettingsFlow({ id: flowId }));
      } catch (loadError) {
        if ([401, 403, 404, 410].includes(responseStatus(loadError) ?? 0)) {
          window.location.replace(settingsEndpoint);
          return;
        }

        setError(
          loadError instanceof Error
            ? loadError.message
            : "Unable to load account settings.",
        );
      }
    };

    void loadFlow();
  }, [flowId]);

  if (error) {
    return <p className="flow-status flow-status--error">{error}</p>;
  }

  if (!flow) {
    return <p className="flow-status">Loading account settings…</p>;
  }

  return (
    <div className="auth-layout">
      <section className="auth-copy" aria-labelledby="settings-copy-title">
        <div className="auth-copy__eyebrow">
          <span aria-hidden="true" />
          ACCOUNT SETTINGS
        </div>
        <h1 id="settings-copy-title">Keep your identity up to date.</h1>
        <p>
          Update your profile or choose a new password through a Kratos
          Settings Flow. Sensitive changes may require a recent sign-in.
        </p>
        <div className="auth-copy__note">
          <span className="auth-copy__note-icon" aria-hidden="true">
            ✓
          </span>
          <span>Credentials are submitted directly to Kratos.</span>
        </div>
      </section>
      <div className="auth-card auth-card--settings">
        <Settings
          flow={flow}
          config={oryConfiguration}
          components={authElementComponents}
        />
      </div>
    </div>
  );
}
