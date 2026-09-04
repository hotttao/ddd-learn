import { useTranslation } from "react-i18next";
import { Link, Route, Routes } from "react-router-dom";
import { getIdentitySummary } from "@/domains/auth/lib/session";
import { useOrySession } from "@/foundation/providers/ory-session";
import AccountSettingsPage from "@/pages/auth/AccountSettingsPage";
import AuthFlowPage from "@/pages/auth/AuthFlowPage";
import RecoveryFlowPage from "@/pages/auth/RecoveryFlowPage";
import SessionPage from "@/pages/auth/SessionPage";
import VerificationFlowPage from "@/pages/auth/VerificationFlowPage";
import XhsConsolePage from "@/pages/xhs/console";

function AuthNavigation() {
  const { t } = useTranslation();
  const { status, session, isLoggingOut, logout } = useOrySession();

  if (status === "authenticated" && session) {
    const identity = getIdentitySummary(session);

    return (
      <nav className="brand-nav" aria-label={t("navigation.currentSession")}>
        <span className="brand-nav__identity">{identity.displayName}</span>
        <Link to="/xhs">{t("navigation.xhsConsole")}</Link>
        <Link to="/settings">{t("navigation.accountSettings")}</Link>
        <button
          className="brand-nav__button"
          type="button"
          disabled={isLoggingOut}
          onClick={() => void logout()}
        >
          {isLoggingOut ? t("navigation.signingOut") : t("navigation.signOut")}
        </button>
      </nav>
    );
  }

  return (
    <nav className="brand-nav" aria-label={t("navigation.authentication")}>
      <Link to="/login">{t("navigation.signIn")}</Link>
      <Link className="brand-nav__button" to="/registration">
        {t("navigation.createAccount")}
      </Link>
    </nav>
  );
}

export default function App() {
  const { t } = useTranslation();

  return (
    <div className="app-shell">
      <header className="brand-bar">
        <Link className="brand-mark" to="/">
          <span className="brand-mark__icon">D</span>
          <span>
            <strong>{t("brand.name")}</strong>
            <small>{t("brand.subtitle")}</small>
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
          <Route path="/recovery" element={<RecoveryFlowPage />} />
          <Route path="/verification" element={<VerificationFlowPage />} />
          <Route path="/settings" element={<AccountSettingsPage />} />
          <Route path="/xhs" element={<XhsConsolePage />} />
          <Route path="*" element={<SessionPage />} />
        </Routes>
      </main>
    </div>
  );
}
