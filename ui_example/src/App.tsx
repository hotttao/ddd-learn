import { Link, Route, Routes } from "react-router-dom";
import { getIdentitySummary } from "@/domains/auth/lib/session";
import { useOrySession } from "@/foundation/providers/ory-session";
import AuthFlowPage from "@/pages/auth/AuthFlowPage";
import SessionPage from "@/pages/auth/SessionPage";

function AuthNavigation() {
  const { status, session, isLoggingOut, logout } = useOrySession();

  if (status === "authenticated" && session) {
    const identity = getIdentitySummary(session);

    return (
      <nav className="brand-nav" aria-label="Current session">
        <span className="brand-nav__identity">{identity.displayName}</span>
        <button
          className="brand-nav__button"
          type="button"
          disabled={isLoggingOut}
          onClick={() => void logout()}
        >
          {isLoggingOut ? "Signing out…" : "Sign out"}
        </button>
      </nav>
    );
  }

  return (
    <nav className="brand-nav" aria-label="Authentication">
      <Link to="/login">Sign in</Link>
      <Link className="brand-nav__button" to="/registration">
        Create account
      </Link>
    </nav>
  );
}

export default function App() {
  return (
    <div className="app-shell">
      <header className="brand-bar">
        <Link className="brand-mark" to="/">
          <span className="brand-mark__icon">D</span>
          <span>
            <strong>DDD Learn</strong>
            <small>Identity Lab</small>
          </span>
        </Link>
        <AuthNavigation />
      </header>
      <main className="app-main auth-theme">
        <Routes>
          <Route path="/" element={<SessionPage />} />
          <Route path="/login" element={<AuthFlowPage kind="login" />} />
          <Route
            path="/registration"
            element={<AuthFlowPage kind="registration" />}
          />
          <Route path="*" element={<SessionPage />} />
        </Routes>
      </main>
    </div>
  );
}
