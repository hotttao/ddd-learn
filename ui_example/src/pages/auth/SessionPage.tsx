import { Link } from "react-router-dom";
import { getIdentitySummary } from "@/domains/auth/lib/session";
import { useOrySession } from "@/foundation/providers/ory-session";

function formatDate(value: Date | undefined): string {
  return value ? value.toLocaleString() : "Not provided";
}

export default function SessionPage() {
  const {
    status,
    session,
    error,
    isLoggingOut,
    refreshSession,
    logout,
  } = useOrySession();

  if (status === "loading") {
    return <p className="flow-status">Checking current session…</p>;
  }

  if (status === "error") {
    return (
      <section className="welcome-card">
        <span className="eyebrow">SESSION CHECK FAILED</span>
        <h1>We could not read your session.</h1>
        <p>{error}</p>
        <button
          className="welcome-card__button"
          type="button"
          onClick={() => void refreshSession()}
        >
          Try again
        </button>
      </section>
    );
  }

  if (status === "anonymous" || !session) {
    return (
      <section className="welcome-card">
        <span className="eyebrow">AUTHENTICATION LAB</span>
        <h1>Build with confidence.</h1>
        <p>Explore secure identity flows powered by Ory Kratos.</p>
        <div className="welcome-card__actions">
          <Link className="welcome-card__button" to="/login">
            Continue to sign in
          </Link>
          <Link
            className="welcome-card__button welcome-card__button--secondary"
            to="/registration"
          >
            Create account
          </Link>
        </div>
      </section>
    );
  }

  const identity = getIdentitySummary(session);

  return (
    <section className="session-card">
      <span className="eyebrow">ACTIVE KRATOS SESSION</span>
      <h1>Welcome, {identity.displayName}.</h1>
      <p>
        Registration and authentication completed successfully. This page is
        reading the browser Session directly from Kratos.
      </p>

      <dl className="session-card__details">
        {identity.email && (
          <div>
            <dt>Email</dt>
            <dd>{identity.email}</dd>
          </div>
        )}
        {identity.phone && (
          <div>
            <dt>Phone</dt>
            <dd>{identity.phone}</dd>
          </div>
        )}
        <div>
          <dt>Identity ID</dt>
          <dd>{session.identity?.id ?? "Not provided"}</dd>
        </div>
        <div>
          <dt>Session ID</dt>
          <dd>{session.id}</dd>
        </div>
        <div>
          <dt>Authenticator level</dt>
          <dd>{session.authenticator_assurance_level ?? "Not provided"}</dd>
        </div>
        <div>
          <dt>Expires at</dt>
          <dd>{formatDate(session.expires_at)}</dd>
        </div>
      </dl>

      <button
        className="welcome-card__button session-card__logout"
        type="button"
        disabled={isLoggingOut}
        onClick={() => void logout()}
      >
        {isLoggingOut ? "Signing out…" : "Sign out"}
      </button>
    </section>
  );
}
