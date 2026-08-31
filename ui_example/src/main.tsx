import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { OrySessionProvider } from "./foundation/providers/ory-session";
import "./styles/tokens.css";
import "./index.css";
import "@ory/elements-react/theme/styles.css";
import "./styles/ory-theme.css";
import "./pages/auth/auth.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Root element #root not found in index.html");
}

ReactDOM.createRoot(rootElement).render(
  <React.StrictMode>
    <BrowserRouter>
      <OrySessionProvider>
        <App />
      </OrySessionProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
