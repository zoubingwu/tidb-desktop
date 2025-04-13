import "./style.css";
import React from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "@/components/ThemeProvider";
import { Toaster } from "@/components/ui/sonner";
import { CheckCircle2, AlertCircle } from "lucide-react";

import App from "./App";

const container = document.getElementById("root");
const root = createRoot(container!);

root.render(
  <React.StrictMode>
    <ThemeProvider>
      <App />
      <Toaster
        icons={{
          success: <CheckCircle2 color="green" size={16} />,
          error: <AlertCircle color="red" size={16} />,
        }}
      />
    </ThemeProvider>
  </React.StrictMode>,
);
