import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useTheme, availableThemes } from "@/components/ThemeProvider";
import { Loader2 } from "lucide-react";

import { GetThemeSettings, SaveThemeSettings } from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";

interface SettingsModalProps {
  children: React.ReactNode; // To wrap the trigger button
}

type ThemeMode = "light" | "dark" | "system"; // Explicit type for mode

export function SettingsModal({ children }: SettingsModalProps) {
  const queryClient = useQueryClient();
  const { setTheme } = useTheme(); // Only need setTheme from next-themes

  // Local state for the UI controls, initialized later by query data
  const [selectedBaseTheme, setSelectedBaseTheme] = useState<string>("");
  const [selectedMode, setSelectedMode] = useState<ThemeMode>("system");

  // Query to fetch initial settings from backend
  const {
    data: savedSettings,
    isLoading: isLoadingSettings,
    isError: isErrorSettings,
  } = useQuery({
    queryKey: ["themeSettings"],
    queryFn: GetThemeSettings,
    staleTime: Infinity, // Config data rarely changes externally
    gcTime: Infinity,
    refetchOnWindowFocus: false,
  });

  // Mutation to save settings to backend
  const { mutate: saveSettings, isPending: isSavingSettings } = useMutation({
    mutationFn: (settings: services.ThemeSettings) =>
      SaveThemeSettings(settings),
    onSuccess: (savedData, variables) => {
      // Update the query cache immediately
      queryClient.setQueryData(["themeSettings"], variables);
      // Apply the theme visually
      applyTheme(variables.baseTheme, variables.mode as ThemeMode);
      console.log("Theme settings saved and applied:", savedData, variables);
    },
    onError: (error) => {
      console.error("Error saving theme settings:", error);
      // Optionally show an error message to the user
    },
  });

  // Effect to initialize local state once settings are loaded
  useEffect(() => {
    if (savedSettings) {
      setSelectedBaseTheme(savedSettings.baseTheme || availableThemes[0]);
      setSelectedMode((savedSettings.mode as ThemeMode) || "system");
      // Apply initial theme when modal loads/settings are fetched
      applyTheme(
        savedSettings.baseTheme || availableThemes[0],
        (savedSettings.mode as ThemeMode) || "system",
      );
    }
  }, [savedSettings, setTheme]); // Dependency on savedSettings

  // Function to apply theme visually (both base and mode)
  const applyTheme = (base: string, mode: ThemeMode) => {
    console.log(`Applying base theme: ${base}, mode: ${mode}`);
    // Remove old base theme class
    document.documentElement.classList.forEach((cls) => {
      if (availableThemes.includes(cls)) {
        document.documentElement.classList.remove(cls);
      }
    });
    // Add new base theme class
    if (base && availableThemes.includes(base)) {
      document.documentElement.classList.add(base);
    }
    // Use next-themes to handle light/dark/system mode
    setTheme(mode);
  };

  // Handler for base theme selection change
  const handleBaseThemeChange = (newBaseTheme: string) => {
    setSelectedBaseTheme(newBaseTheme);
    // Immediately save the change
    saveSettings({ baseTheme: newBaseTheme, mode: selectedMode });
  };

  // Handler for mode selection change
  const handleModeChange = (newMode: ThemeMode) => {
    setSelectedMode(newMode);
    // Immediately save the change
    saveSettings({ baseTheme: selectedBaseTheme, mode: newMode });
  };

  // Determine if the UI should be disabled
  const isBusy = isLoadingSettings || isSavingSettings;

  return (
    <Dialog>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
        </DialogHeader>
        {isLoadingSettings ? (
          <div className="flex items-center justify-center p-10">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" /> Loading
            settings...
          </div>
        ) : isErrorSettings ? (
          <div className="text-destructive p-4 text-center">
            Error loading settings. Please try again.
          </div>
        ) : (
          <div className="grid gap-6 py-4">
            {/* Mode Selection */}
            <fieldset
              disabled={isBusy}
              className="grid grid-cols-4 items-center gap-4"
            >
              <Label htmlFor="theme-mode" className="text-right col-span-1">
                Mode
              </Label>
              <RadioGroup
                value={selectedMode}
                onValueChange={(value) => handleModeChange(value as ThemeMode)}
                className="col-span-3 flex space-x-2"
                id="theme-mode"
              >
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="light" id="r-light" />
                  <Label htmlFor="r-light">Light</Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="dark" id="r-dark" />
                  <Label htmlFor="r-dark">Dark</Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="system" id="r-system" />
                  <Label htmlFor="r-system">System</Label>
                </div>
              </RadioGroup>
            </fieldset>

            {/* Base Theme Selection */}
            <fieldset
              disabled={isBusy}
              className="grid grid-cols-4 items-center gap-4"
            >
              <Label htmlFor="base-theme" className="text-right">
                Theme
              </Label>
              <Select
                value={selectedBaseTheme}
                onValueChange={handleBaseThemeChange}
                disabled={isBusy} // Also disable select directly
              >
                <SelectTrigger className="col-span-3" id="base-theme">
                  <SelectValue placeholder="Select a theme" />
                </SelectTrigger>
                <SelectContent>
                  {availableThemes.map((baseTheme) => (
                    <SelectItem key={baseTheme} value={baseTheme}>
                      {baseTheme.charAt(0).toUpperCase() + baseTheme.slice(1)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </fieldset>
            {/* Add other settings here */}
          </div>
        )}
        <DialogFooter>
          {isSavingSettings && (
            <span className="text-sm text-muted-foreground mr-auto flex items-center">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Saving...
            </span>
          )}
          <DialogClose asChild>
            {/* Disable close button while saving? Maybe not necessary */}
            <Button type="button" variant="secondary">
              Close
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
