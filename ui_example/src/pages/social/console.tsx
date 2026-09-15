import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useOrySession } from "@/foundation/providers/ory-session";
import { listMyOrganizations, searchContents, SocialApiError } from "@/domains/social/api";
import type { OrganizationMembership, SocialContent } from "@/domains/social/types";
import "./social.css";

export default function SocialConsolePage() {
  const { status, session } = useOrySession();
  const [organizations, setOrganizations] = useState<OrganizationMembership[]>([]);
  const [organizationId, setOrganizationId] = useState("");
  const [keyword, setKeyword] = useState("golang");
  const [faultMode, setFaultMode] = useState<"" | "unavailable" | "delay">("");
  const [contents, setContents] = useState<SocialContent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (status !== "authenticated" || !session) return;
    void listMyOrganizations()
      .then((response) => {
        const items = response.data.organizations ?? [];
        setOrganizations(items);
        setOrganizationId(items[0]?.id ?? "");
      })
      .catch((reason: unknown) => {
        setError(reason instanceof Error ? reason.message : String(reason));
      });
  }, [status, session]);

  if (status === "loading") return <p className="flow-status">Checking current session…</p>;
  if (status !== "authenticated" || !session) {
    return (
      <section className="welcome-card">
        <span className="eyebrow">AUTHENTICATION REQUIRED</span>
        <h1>Sign in before testing Social.</h1>
        <p>Social requests are protected by Oathkeeper.</p>
        <Link className="welcome-card__button" to="/login">Continue to sign in</Link>
      </section>
    );
  }

  const search = () => {
    if (!organizationId) return;
    setLoading(true);
    setError(null);
    void searchContents(organizationId, keyword, faultMode)
      .then((response) => setContents(response.data.contents ?? []))
      .catch((reason: unknown) => {
        setContents([]);
        setError(reason instanceof SocialApiError ? `${reason.status}: ${reason.message}` : String(reason));
      })
      .finally(() => setLoading(false));
  };

  return (
    <section className="social-console">
      <span className="eyebrow">PROXYLESS SOCIAL LAB</span>
      <h1>Social content search</h1>
      <p>Call the Social aggregation service through Istio Gateway and gRPC-JSON Transcoder.</p>
      <label>
        Organization
        <select value={organizationId} onChange={(event) => setOrganizationId(event.target.value)}>
          {organizations.map((item) => <option key={item.id} value={item.id}>{item.id} ({item.roles?.join(", ")})</option>)}
        </select>
      </label>
      <label>
        Keyword
        <input value={keyword} onChange={(event) => setKeyword(event.target.value)} />
      </label>
      <label>
        Test fault mode
        <select value={faultMode} onChange={(event) => setFaultMode(event.target.value as typeof faultMode)}>
          <option value="">Disabled</option>
          <option value="unavailable">Unavailable</option>
          <option value="delay">6-second delay</option>
        </select>
      </label>
      <button type="button" disabled={loading || !organizationId} onClick={search}>
        {loading ? "Searching…" : "Search contents"}
      </button>
      {error && <p className="social-console__error">{error}</p>}
      <div className="social-console__results">
        {contents.map((item) => (
          <article key={item.id}>
            <span>{item.platform}</span>
            <h2>{item.title}</h2>
            <p>Keyword: {item.source_keyword}</p>
          </article>
        ))}
        {!loading && !error && contents.length === 0 && <p>No contents returned.</p>}
      </div>
    </section>
  );
}
