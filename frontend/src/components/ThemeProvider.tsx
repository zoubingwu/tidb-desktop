import {
  createContext,
  useContext,
  useState,
  useEffect,
  type ReactNode,
} from "react";
import { useQuery } from "@tanstack/react-query";
import { GetThemeSettings, SaveThemeSettings } from "wailsjs/go/main/App";

// Define the available theme names. These should match CSS classes.
// These are the *base* themes you switch between.
export const availableThemes = [
  "claude", // Assuming 'claude' is the default/base theme (maps to :root)
  "nature",
  "elegant-luxury",
  "neo-brutalism",
  "quantum-rose",
  "sunset-horizon",
];

export type ThemeMode = "light" | "dark" | "system";

interface ThemeContextType {
  baseTheme: string;
  mode: ThemeMode;
  setBaseTheme: (theme: string) => void;
  setMode: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

// Helper to apply theme classes to the root element
export const applyTheme = (base: string, mode: ThemeMode) => {
  console.log(`Applying base theme: ${base}, mode: ${mode}`);
  const root = document.documentElement;

  // Remove old base theme class
  root.classList.forEach((cls) => {
    if (availableThemes.includes(cls)) {
      root.classList.remove(cls);
    }
  });

  // Add new base theme class
  if (base && availableThemes.includes(base)) {
    root.classList.add(base);
  }

  // Handle dark/light mode
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  if (mode === "dark" || (mode === "system" && prefersDark)) {
    root.classList.add("dark");
  } else {
    root.classList.remove("dark");
  }
};

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [baseTheme, setBaseThemeState] = useState<string>(availableThemes[0]);
  const [mode, setModeState] = useState<ThemeMode>("system");

  // Query to fetch initial settings from backend
  const { data: savedSettings } = useQuery({
    queryKey: ["themeSettings"],
    queryFn: GetThemeSettings,
    refetchOnWindowFocus: false,
  });

  // Apply saved settings on initial load
  useEffect(() => {
    if (savedSettings) {
      const initialBase = savedSettings.baseTheme || availableThemes[0];
      const initialMode = (savedSettings.mode as ThemeMode) || "system";
      setBaseThemeState(initialBase);
      setModeState(initialMode);
      // Initial apply is handled by the effect below
    }
  }, [savedSettings]);

  // Apply theme whenever baseTheme or mode changes
  useEffect(() => {
    applyTheme(baseTheme, mode);
  }, [baseTheme, mode]);

  // Listen for system theme changes when mode is 'system'
  useEffect(() => {
    if (mode !== "system") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = () => {
      applyTheme(baseTheme, "system"); // Re-apply with 'system' to check preference
    };

    mediaQuery.addEventListener("change", handleChange);
    return () => mediaQuery.removeEventListener("change", handleChange);
  }, [mode, baseTheme]);

  // Persist theme changes to backend
  const persistTheme = async (newBase: string, newMode: ThemeMode) => {
    try {
      // Assuming a SaveThemeSettings function exists in your Go backend
      await SaveThemeSettings({ baseTheme: newBase, mode: newMode });
      console.log("Theme settings saved:", {
        baseTheme: newBase,
        mode: newMode,
      });
    } catch (error) {
      console.error("Failed to save theme settings:", error);
    }
  };

  const setBaseTheme = (theme: string) => {
    if (availableThemes.includes(theme)) {
      setBaseThemeState(theme);
      persistTheme(theme, mode);
    } else {
      console.warn(`Attempted to set invalid base theme: ${theme}`);
    }
  };

  const setMode = (newMode: ThemeMode) => {
    setModeState(newMode);
    persistTheme(baseTheme, newMode);
  };

  return (
    <ThemeContext.Provider value={{ baseTheme, mode, setBaseTheme, setMode }}>
      {children}
    </ThemeContext.Provider>
  );
}

// Custom hook to use the theme context
export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return context;
};
