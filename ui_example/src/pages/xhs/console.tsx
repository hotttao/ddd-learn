import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  listCrawlContents,
  listMyOrganizations,
  startCrawlTask,
  updateKeywords,
  XhsApiError,
} from "@/domains/xhs/lib/api";
import type {
  OrganizationMembership,
  XhsRequestResult,
} from "@/domains/xhs/types";
import { useOrySession } from "@/foundation/providers/ory-session";
import "./xhs.css";

type Operation = "contents" | "keywords" | "task";

type DisplayResult = {
  body: unknown;
  method?: string;
  path?: string;
  status?: number;
  title: string;
};

function parseKeywords(value: string): string[] {
  return value
    .split(/[\n,，]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export default function XhsConsolePage() {
  const { t } = useTranslation();
  const { status: sessionStatus, session } = useOrySession();
  const [organizations, setOrganizations] = useState<OrganizationMembership[]>(
    [],
  );
  const [organizationId, setOrganizationId] = useState("");
  const [organizationsLoading, setOrganizationsLoading] = useState(false);
  const [taskKeywords, setTaskKeywords] = useState("Ory, Hertz");
  const [savedKeywords, setSavedKeywords] = useState("Kratos, Oathkeeper");
  const [running, setRunning] = useState<Operation | null>(null);
  const [result, setResult] = useState<DisplayResult | null>(null);

  useEffect(() => {
    if (sessionStatus !== "authenticated" || !session) {
      setOrganizations([]);
      setOrganizationId("");
      return;
    }

    let cancelled = false;
    setOrganizationsLoading(true);
    void listMyOrganizations()
      .then((response) => {
        if (cancelled) return;
        const memberships = response.data.organizations ?? [];
        setOrganizations(memberships);
        setOrganizationId(memberships[0]?.id ?? "");
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setOrganizations([]);
        setOrganizationId("");
        if (error instanceof XhsApiError) {
          setResult({
            title: t("xhs.organizationLoadFailed"),
            method: error.method,
            path: error.path,
            status: error.status,
            body: error.body,
          });
        }
      })
      .finally(() => {
        if (!cancelled) setOrganizationsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [sessionStatus, session, t]);

  const run = async <T,>(
    operation: Operation,
    title: string,
    action: () => Promise<XhsRequestResult<T>>,
  ) => {
    if (!organizationId.trim()) {
      setResult({
        title,
        body: { error: t("xhs.validation.organizationRequired") },
      });
      return;
    }

    setRunning(operation);
    try {
      const response = await action();
      setResult({
        title,
        method: response.method,
        path: response.path,
        status: response.status,
        body: response.data,
      });
    } catch (error) {
      if (error instanceof XhsApiError) {
        setResult({
          title,
          method: error.method,
          path: error.path,
          status: error.status,
          body: error.body,
        });
      } else {
        setResult({
          title,
          body: {
            error: error instanceof Error ? error.message : String(error),
          },
        });
      }
    } finally {
      setRunning(null);
    }
  };

  if (sessionStatus === "loading") {
    return <p className="flow-status">{t("xhs.sessionChecking")}</p>;
  }

  if (sessionStatus !== "authenticated" || !session) {
    return (
      <section className="welcome-card">
        <span className="eyebrow">{t("xhs.authenticationRequired")}</span>
        <h1>{t("xhs.signInTitle")}</h1>
        <p>{t("xhs.signInDescription")}</p>
        <Link className="welcome-card__button" to="/login">
          {t("xhs.signIn")}
        </Link>
      </section>
    );
  }

  return (
    <section className="xhs-console">
      <header className="xhs-console__header">
        <div>
          <span className="eyebrow">{t("xhs.eyebrow")}</span>
          <h1>{t("xhs.title")}</h1>
          <p>{t("xhs.description")}</p>
        </div>
        <div className="xhs-console__session">
          <span>{t("xhs.sessionActive")}</span>
          <code>{session.id}</code>
        </div>
      </header>

      <label className="xhs-console__organization">
        <span>{t("xhs.organization")}</span>
        <select
          value={organizationId}
          onChange={(event) => setOrganizationId(event.target.value)}
          disabled={organizationsLoading || organizations.length === 0}
        >
          {organizationId === "" && (
            <option value="">
              {organizationsLoading
                ? t("xhs.organizationLoading")
                : t("xhs.organizationEmpty")}
            </option>
          )}
          {organizations.map((organization) => (
            <option key={organization.id} value={organization.id}>
              {organization.id} ({organization.roles?.join(", ")})
            </option>
          ))}
        </select>
        <small>{t("xhs.organizationHint")}</small>
      </label>

      <div className="xhs-console__operations">
        <article className="xhs-operation">
          <span className="xhs-operation__method xhs-operation__method--post">
            POST
          </span>
          <h2>{t("xhs.startTask.title")}</h2>
          <p>{t("xhs.startTask.description")}</p>
          <textarea
            value={taskKeywords}
            onChange={(event) => setTaskKeywords(event.target.value)}
            aria-label={t("xhs.startTask.keywords")}
          />
          <button
            type="button"
            disabled={running !== null || organizationId === ""}
            onClick={() =>
              void run("task", t("xhs.startTask.result"), () =>
                startCrawlTask(
                  organizationId.trim(),
                  parseKeywords(taskKeywords),
                ),
              )
            }
          >
            {running === "task"
              ? t("xhs.requesting")
              : t("xhs.startTask.action")}
          </button>
        </article>

        <article className="xhs-operation">
          <span className="xhs-operation__method xhs-operation__method--get">
            GET
          </span>
          <h2>{t("xhs.contents.title")}</h2>
          <p>{t("xhs.contents.description")}</p>
          <div className="xhs-operation__spacer" />
          <button
            type="button"
            disabled={running !== null || organizationId === ""}
            onClick={() =>
              void run("contents", t("xhs.contents.result"), () =>
                listCrawlContents(organizationId.trim()),
              )
            }
          >
            {running === "contents"
              ? t("xhs.requesting")
              : t("xhs.contents.action")}
          </button>
        </article>

        <article className="xhs-operation">
          <span className="xhs-operation__method xhs-operation__method--put">
            PUT
          </span>
          <h2>{t("xhs.keywords.title")}</h2>
          <p>{t("xhs.keywords.description")}</p>
          <textarea
            value={savedKeywords}
            onChange={(event) => setSavedKeywords(event.target.value)}
            aria-label={t("xhs.keywords.values")}
          />
          <button
            type="button"
            disabled={running !== null || organizationId === ""}
            onClick={() =>
              void run("keywords", t("xhs.keywords.result"), () =>
                updateKeywords(
                  organizationId.trim(),
                  parseKeywords(savedKeywords),
                ),
              )
            }
          >
            {running === "keywords"
              ? t("xhs.requesting")
              : t("xhs.keywords.action")}
          </button>
        </article>
      </div>

      <section className="xhs-response" aria-live="polite">
        <div className="xhs-response__heading">
          <div>
            <span className="eyebrow">{t("xhs.response.eyebrow")}</span>
            <h2>{result?.title ?? t("xhs.response.waiting")}</h2>
          </div>
          {result?.status && (
            <span
              className={`xhs-response__status ${result.status >= 400 ? "xhs-response__status--error" : ""}`}
            >
              HTTP {result.status}
            </span>
          )}
        </div>
        {result?.path && (
          <code className="xhs-response__request">
            {result.method} {result.path}
          </code>
        )}
        <pre>
          {JSON.stringify(
            result?.body ?? { message: t("xhs.response.hint") },
            null,
            2,
          )}
        </pre>
      </section>
    </section>
  );
}
