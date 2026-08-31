import { useEffect, useState } from "react";
import type { SettingsFlow } from "@ory/client-fetch";
import { Settings } from "@ory/elements-react/theme";
import { oryClient, oryConfiguration } from "@/foundation/lib/ory";
import { authElementComponents } from "./ory-components";

export default function RecoverySettingsPage() {
  const [flow, setFlow] = useState<SettingsFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const flowId = new URLSearchParams(window.location.search).get("flow");

  useEffect(() => {
    if (!flowId) {
      window.location.replace("/");
      return;
    }

    const loadFlow = async () => {
      try {
        setFlow(await oryClient.getSettingsFlow({ id: flowId }));
      } catch (loadError) {
        setError(
          loadError instanceof Error
            ? loadError.message
            : "Unable to load password settings.",
        );
      }
    };

    void loadFlow();
  }, [flowId]);

  if (error) {
    return <p className="flow-status flow-status--error">{error}</p>;
  }

  if (!flow) {
    return <p className="flow-status">Loading password settings…</p>;
  }

  return (
    <div className="auth-layout">
      <section className="auth-copy" aria-labelledby="settings-copy-title">
        <div className="auth-copy__eyebrow">
          <span aria-hidden="true" />
          RECOVERY VERIFIED
        </div>
        <h1 id="settings-copy-title">Choose a new password.</h1>
        <p>
          Kratos accepted the recovery code and issued this privileged Settings
          Flow. The new password is submitted directly to Kratos.
        </p>
        <div className="auth-copy__note">
          <span className="auth-copy__note-icon" aria-hidden="true">
            ✓
          </span>
          <span>The recovery code itself cannot be reused as a password.</span>
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
