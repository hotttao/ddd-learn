import { Configuration, FrontendApi } from "@ory/client-fetch";
import type { OryClientConfiguration } from "@ory/elements-react";

export const KRATOS_PUBLIC_URL =
  import.meta.env.VITE_KRATOS_PUBLIC_URL ?? "/kratos";

export const oryClient = new FrontendApi(
  new Configuration({
    basePath: KRATOS_PUBLIC_URL,
    credentials: "include",
  }),
);

export const oryConfiguration = {
  sdk: {
    url: KRATOS_PUBLIC_URL,
    options: { credentials: "include" },
  },
  project: {
    name: "DDD Learn Auth",
    default_redirect_url: "/",
    error_ui_url: "/error",
    login_ui_url: "/login",
    registration_ui_url: "/registration",
    recovery_ui_url: "/recovery",
    verification_ui_url: "/verification",
    settings_ui_url: "/settings",
    registration_enabled: true,
    recovery_enabled: true,
    verification_enabled: true,
  },
} satisfies OryClientConfiguration;
