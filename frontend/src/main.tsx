import "./style.css";
import React from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "@/components/ThemeProvider";
import { Toaster } from "@/components/ui/sonner";
import { CheckCircle2, Info, AlertCircle, Loader2 } from "lucide-react";

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
          info: <Info color="blue" size={16} />,
          warning: <AlertCircle color="yellow" size={16} />,
          error: <AlertCircle color="red" size={16} />,
          loading: <Loader2 className="animate-spin" color="gray" size={16} />,
        }}
      />
    </ThemeProvider>
  </React.StrictMode>,
);
