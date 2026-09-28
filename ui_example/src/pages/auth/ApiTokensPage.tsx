import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useOrySession } from "@/foundation/providers/ory-session";
import {
  ApiTokenError,
  createApiToken,
  listApiTokens,
  revokeApiToken,
  rotateApiToken,
  type ApiToken,
} from "@/domains/auth/lib/api-token";
import "./api-tokens.css";

function formatDate(value: string): string {
  return new Date(value).toLocaleString();
}

export default function ApiTokensPage() {
  const { status, session } = useOrySession();
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [name, setName] = useState("cli-token");
  const [scopes, setScopes] = useState("xhs.read");
  const [ttl, setTtl] = useState("720h");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const load = () => {
    setBusy(true);
    setError("");
    void listApiTokens()
      .then((response) => setTokens(response.tokens ?? []))
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : String(reason)))
      .finally(() => setBusy(false));
  };

  useEffect(() => {
    if (status === "authenticated" && session) load();
  }, [status, session]);

  const issue = () => {
    setBusy(true);
    setError("");
    setSecret("");
    void createApiToken({
      name: name.trim(),
      scopes: scopes.split(/[\n,，]/).map((item) => item.trim()).filter(Boolean),
      ttl: ttl.trim(),
    })
      .then((response) => {
        setSecret(response.secret ?? "");
        load();
      })
      .catch((reason: unknown) => setError(reason instanceof ApiTokenError ? reason.message : String(reason)))
      .finally(() => setBusy(false));
  };

  const revoke = (id: string) => {
    setBusy(true);
    void revokeApiToken(id).then(load).catch((reason: unknown) => setError(String(reason))).finally(() => setBusy(false));
  };

  const rotate = (id: string) => {
    setBusy(true);
    setSecret("");
    void rotateApiToken(id)
      .then((response) => {
        setSecret(response.secret ?? "");
        load();
      })
      .catch((reason: unknown) => setError(String(reason)))
      .finally(() => setBusy(false));
  };

  if (status === "loading") return <p className="flow-status">Checking current session…</p>;
  if (status !== "authenticated" || !session) {
    return (
      <section className="welcome-card">
        <span className="eyebrow">AUTHENTICATION REQUIRED</span>
        <h1>Sign in to manage API Tokens.</h1>
        <p>API Tokens belong to the current Kratos identity.</p>
        <Link className="welcome-card__button" to="/login">Continue to sign in</Link>
      </section>
    );
  }

  return (
    <section className="api-tokens-page">
      <header>
        <span className="eyebrow">API TOKEN MANAGEMENT</span>
        <h1>CLI and service access</h1>
        <p>Issue a long-lived API Token for a CLI or internal service. The secret is shown only once.</p>
      </header>
      <div className="api-token-form">
        <label>Token name<input value={name} onChange={(event) => setName(event.target.value)} /></label>
        <label>Scopes<input value={scopes} onChange={(event) => setScopes(event.target.value)} /></label>
        <label>TTL<input value={ttl} onChange={(event) => setTtl(event.target.value)} /></label>
        <button type="button" disabled={busy || !name.trim()} onClick={issue}>Issue API Token</button>
      </div>
      {secret && <aside className="api-token-secret"><strong>Copy this secret now</strong><p>It will not be shown again and is not stored in browser storage.</p><code>{secret}</code></aside>}
      {error && <p className="api-token-error">{error}</p>}
      <div className="api-token-list">
        <div className="api-token-list__heading"><h2>Your API Tokens</h2><button type="button" disabled={busy} onClick={load}>Refresh</button></div>
        {tokens.length === 0 && <p>No API Tokens yet.</p>}
        {tokens.map((token) => (
          <article key={token.id} className="api-token-card">
            <div><strong>{token.name}</strong><span>{token.status}</span><small>{token.scopes.join(", ") || "No scopes"}</small></div>
            <div><small>Expires {formatDate(token.expire_time)}</small><button type="button" disabled={busy || token.status.includes("REVOKED")} onClick={() => rotate(token.id)}>Rotate</button><button type="button" disabled={busy || token.status.includes("REVOKED")} onClick={() => revoke(token.id)}>Revoke</button></div>
          </article>
        ))}
      </div>
    </section>
  );
}
