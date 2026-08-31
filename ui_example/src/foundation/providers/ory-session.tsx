import type { FrontendApi, Session } from "@ory/client-fetch";
import {
  createContext,
  type PropsWithChildren,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { oryClient } from "@/foundation/lib/ory";

type SessionStatus = "loading" | "authenticated" | "anonymous" | "error";

type OrySessionClient = Pick<
  FrontendApi,
  "toSession" | "createBrowserLogoutFlow"
>;

type OrySessionContextValue = {
  status: SessionStatus;
  session: Session | null;
  error: string | null;
  isLoggingOut: boolean;
  refreshSession: () => Promise<void>;
  logout: () => Promise<void>;
};

type OrySessionProviderProps = PropsWithChildren<{
  client?: OrySessionClient;
}>;

const OrySessionContext = createContext<OrySessionContextValue | null>(null);

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

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export function OrySessionProvider({
  children,
  client = oryClient,
}: OrySessionProviderProps) {
  const [status, setStatus] = useState<SessionStatus>("loading");
  const [session, setSession] = useState<Session | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoggingOut, setIsLoggingOut] = useState(false);

  const refreshSession = useCallback(async () => {
    setStatus("loading");
    setError(null);

    try {
      const currentSession = await client.toSession();
      setSession(currentSession);
      setStatus("authenticated");
    } catch (sessionError) {
      setSession(null);

      if (responseStatus(sessionError) === 401) {
        setStatus("anonymous");
        return;
      }

      setError(
        errorMessage(sessionError, "Unable to check the current session."),
      );
      setStatus("error");
    }
  }, [client]);

  useEffect(() => {
    void refreshSession();
  }, [refreshSession]);

  const logout = useCallback(async () => {
    setIsLoggingOut(true);
    setError(null);

    try {
      const logoutFlow = await client.createBrowserLogoutFlow();
      window.location.assign(logoutFlow.logout_url);
    } catch (logoutError) {
      setError(errorMessage(logoutError, "Unable to start the logout flow."));
      setStatus("error");
      setIsLoggingOut(false);
    }
  }, [client]);

  const value = useMemo<OrySessionContextValue>(
    () => ({
      status,
      session,
      error,
      isLoggingOut,
      refreshSession,
      logout,
    }),
    [error, isLoggingOut, logout, refreshSession, session, status],
  );

  return (
    <OrySessionContext.Provider value={value}>
      {children}
    </OrySessionContext.Provider>
  );
}

export function useOrySession(): OrySessionContextValue {
  const context = useContext(OrySessionContext);
  if (!context) {
    throw new Error("useOrySession must be used inside OrySessionProvider");
  }

  return context;
}
