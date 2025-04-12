"use client"; // Required directive for next-themes components

import { ThemeProvider as NextThemesProvider } from "next-themes";
import { type ThemeProviderProps } from "next-themes";

// Define the available theme names. These should match CSS classes.
// These are the *base* themes you switch between.
// Light/dark is handled automatically by next-themes.
export const availableThemes = [
  "claude", // Assuming 'claude' is the default/base theme (maps to :root)
  "nature",
  "elegant-luxury",
  "neo-brutalism",
  "quantum-rose",
  "sunset-horizon",
];

// You might not need to expose the type `BaseThemeName` anymore
// export type BaseThemeName = (typeof availableThemes)[number];

export function ThemeProvider({ children, ...props }: ThemeProviderProps) {
  return (
    <NextThemesProvider
      attribute="class" // Apply theme class to the html element
      defaultTheme="claude" // Set your default base theme here
      // You can explicitly list themes if needed for validation,
      // but next-themes often works without it if classes match.
      // themes={availableThemes}
      enableSystem // Allows automatic light/dark mode based on system
      // disableTransitionOnChange // Optional: prevent flash on theme change
      {...props}
    >
      {children}
    </NextThemesProvider>
  );
}

// Optional: Re-export useTheme hook from next-themes for consistency
// if you prefer importing from one place.
export { useTheme } from "next-themes";
