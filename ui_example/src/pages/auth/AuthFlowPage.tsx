import { useEffect, useState } from "react";
import type { LoginFlow, RegistrationFlow } from "@ory/client-fetch";
import { Login, Registration } from "@ory/elements-react/theme";
import {
  KRATOS_PUBLIC_URL,
  oryClient,
  oryConfiguration,
} from "@/foundation/lib/ory";
import { authElementComponents } from "./ory-components";

type AuthFlowKind = "login" | "registration";
type AuthFlow = LoginFlow | RegistrationFlow;

type AuthFlowPageProps = {
  kind: AuthFlowKind;
};

const flowEndpoint = (kind: AuthFlowKind) =>
  `${KRATOS_PUBLIC_URL}/self-service/${kind}/browser`;

const copyByKind = {
  login: {
    eyebrow: "WELCOME BACK",
    title: "Your identity, thoughtfully protected.",
    description:
      "Sign in to continue exploring the DDD Learn identity lab with a calm, secure experience.",
  },
  registration: {
    eyebrow: "START YOUR JOURNEY",
    title: "Build your identity foundation.",
    description:
      "Create an account and see how a production-minded authentication flow comes together.",
  },
} as const;

export default function AuthFlowPage({ kind }: AuthFlowPageProps) {
  const [flow, setFlow] = useState<AuthFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const flowId = new URLSearchParams(window.location.search).get("flow");

  useEffect(() => {
    if (!flowId) {
      window.location.replace(flowEndpoint(kind));
      return;
    }

    const loadFlow = async () => {
      try {
        const loadedFlow = kind === "login"
          ? await oryClient.getLoginFlow({ id: flowId })
          : await oryClient.getRegistrationFlow({ id: flowId });
        setFlow(loadedFlow);
      } catch (loadError) {
        setError(
          loadError instanceof Error
            ? loadError.message
            : "Unable to load the authentication flow.",
        );
      }
    };

    void loadFlow();
  }, [flowId, kind]);

  if (error) {
    return <p className="flow-status flow-status--error">{error}</p>;
  }

  if (!flow) {
    return <p className="flow-status">Loading authentication flow…</p>;
  }

  const copy = copyByKind[kind];

  return kind === "login" ? (
    <div className="auth-layout">
      <section className="auth-copy" aria-labelledby="auth-copy-title">
        <div className="auth-copy__eyebrow">
          <span aria-hidden="true" />
          {copy.eyebrow}
        </div>
        <h1 id="auth-copy-title">{copy.title}</h1>
        <p>{copy.description}</p>
        <div className="auth-copy__note">
          <span className="auth-copy__note-icon" aria-hidden="true">
            ✓
          </span>
          <span>Powered by Ory Kratos and designed for clarity.</span>
        </div>
      </section>
      <div className="auth-card">
        <Login
          flow={flow}
          config={oryConfiguration}
          components={authElementComponents}
        />
      </div>
    </div>
  ) : (
    <div className="auth-layout">
      <section className="auth-copy" aria-labelledby="auth-copy-title">
        <div className="auth-copy__eyebrow">
          <span aria-hidden="true" />
          {copy.eyebrow}
        </div>
        <h1 id="auth-copy-title">{copy.title}</h1>
        <p>{copy.description}</p>
        <div className="auth-copy__note">
          <span className="auth-copy__note-icon" aria-hidden="true">
            ✓
          </span>
          <span>One account, ready for every service in the lab.</span>
        </div>
      </section>
      <div className="auth-card">
        <Registration
          flow={flow}
          config={oryConfiguration}
          components={authElementComponents}
        />
      </div>
    </div>
  );
}
