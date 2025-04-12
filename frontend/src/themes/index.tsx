import React, {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
} from "react";

// --- Configuration ---
// Define the available theme names. These should match CSS classes you'll create.
export const availableThemes = [
  "nature",
  "claude", // Assuming 'claude' is the default/base
  "elegant-luxury",
  "neo-brutalism",
  "quantum-rose",
  "sunset-horizon",
] as const; // Add/remove themes based on your @import in style.css

// Derive the type for a specific theme name
export type BaseThemeName = (typeof availableThemes)[number];

// ThemeSelection now only allows specific theme names
export type ThemeSelection = BaseThemeName;

// Set a default theme from the available list
const defaultThemeSelection: ThemeSelection = "claude"; // Or choose another default
const storageKey = "tidb-desktop-theme"; // Unique key for localStorage

// --- Types for Context ---
type ThemeProviderState = {
  themeSelection: ThemeSelection; // The user's preference ('claude', 'nature', etc.)
  resolvedTheme: "light" | "dark"; // The actual mode applied ('light' or 'dark')
  setTheme: (theme: ThemeSelection) => void; // Function to change the theme selection
};

// --- Context ---
const ThemeProviderContext = createContext<ThemeProviderState | undefined>(
  undefined,
);

// --- Theme Provider Component ---
type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: ThemeSelection;
  storageKey?: string;
};

export function ThemeProvider({
  children,
  defaultTheme: initialDefaultTheme = defaultThemeSelection,
  storageKey: key = storageKey,
}: ThemeProviderProps) {
  // State holds the user's *selected* theme preference
  const [themeSelection, setThemeSelectionState] = useState<ThemeSelection>(
    () => (localStorage.getItem(key) as ThemeSelection) || initialDefaultTheme,
  );

  // State holds the *actual* applied mode (light or dark)
  const [resolvedTheme, setResolvedTheme] = useState<"light" | "dark">("light");

  // Function to apply theme classes to the root element
  const applyTheme = useCallback(
    (selected: ThemeSelection) => {
      const root = window.document.documentElement; // Apply to <html>

      // 1. Determine Light/Dark Mode based *only* on system preference
      const systemPrefersDark = window.matchMedia(
        "(prefers-color-scheme: dark)",
      ).matches;
      const currentMode: "light" | "dark" = systemPrefersDark
        ? "dark"
        : "light";

      // 2. Clear previous classes
      // Remove 'light', 'dark', and all potential base theme classes
      root.classList.remove("light", "dark", ...availableThemes);

      // 3. Add new classes
      root.classList.add(selected);

      // Add the determined light or dark mode class
      root.classList.add(currentMode);

      // 4. Update state and storage
      setResolvedTheme(currentMode);
      localStorage.setItem(key, selected);
    },
    [key],
  ); // Dependency: storage key

  // Effect 1: Apply theme when the component mounts or selection changes
  useEffect(() => {
    applyTheme(themeSelection);
  }, [themeSelection, applyTheme]);

  // Effect 2: Listen for system theme changes to update light/dark mode
  useEffect(() => {
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");

    // Re-apply the *currently selected* theme when system preference changes
    const handleSystemChange = () => applyTheme(themeSelection);

    mediaQuery.addEventListener("change", handleSystemChange);
    // Cleanup listener when component unmounts
    return () => mediaQuery.removeEventListener("change", handleSystemChange);
    // Dependency includes themeSelection to re-apply the correct theme class
  }, [themeSelection, applyTheme]);

  // Function provided to consumers to update the theme selection
  const setTheme = (newTheme: ThemeSelection) => {
    setThemeSelectionState(newTheme); // Trigger state update -> triggers effect 1
  };

  // Value provided by the context
  const value: ThemeProviderState = {
    themeSelection,
    resolvedTheme,
    setTheme,
  };

  return (
    <ThemeProviderContext.Provider value={value}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

// --- useTheme Hook ---
export const useTheme = (): ThemeProviderState => {
  const context = useContext(ThemeProviderContext);
  if (context === undefined) {
    // Ensures the hook is used within the provider's component tree
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return context;
};
