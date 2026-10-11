import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { Toaster } from "sonner";
import { App } from "./routes/app";
import "./styles.css";

const client = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 10000, retry: 1, refetchOnWindowFocus: true },
    mutations: { retry: false },
  },
});
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
      <Toaster richColors position="bottom-right" />
    </QueryClientProvider>
  </React.StrictMode>,
);
