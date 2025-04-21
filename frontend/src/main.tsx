import "./style.css";
import React from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { CheckCircle2, AlertCircle } from "lucide-react";
import { ThemeProvider } from "@/components/ThemeProvider";
import { Toaster } from "@/components/ui/sonner";
import ErrorBoundary from "@/components/ErrorBoundary";
import App from "./App";
import { queryClient } from "./query-client";

const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <ErrorBoundary>
          <App />
        </ErrorBoundary>
        <Toaster
          icons={{
            success: <CheckCircle2 color="green" size={16} />,
            error: <AlertCircle color="red" size={16} />,
          }}
        />
      </ThemeProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
