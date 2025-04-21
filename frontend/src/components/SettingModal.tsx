import {
  type ThemeMode,
  availableThemes,
  useTheme,
} from "@/components/ThemeProvider";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { TooltipTrigger } from "@/components/ui/tooltip";
import { capitalize } from "@/lib/utils";
import { memo, useEffect, useState } from "react";
import {
  GetAIProviderSettings,
  SaveAIProviderSettings,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";

// Local types mirroring the *data structure* of Go types, excluding methods
interface LocalOpenAISettings {
  apiKey?: string;
  baseURL?: string;
}
interface LocalAnthropicSettings {
  apiKey?: string;
  baseURL?: string;
}
interface LocalOpenRouterSettings {
  apiKey?: string;
}
interface LocalAIProviderSettings {
  provider?: AIProvider;
  openai?: LocalOpenAISettings;
  anthropic?: LocalAnthropicSettings;
  openrouter?: LocalOpenRouterSettings;
}

// Define available providers
const aiProviders = ["openai", "anthropic", "openrouter"] as const;
type AIProvider = (typeof aiProviders)[number];

interface SettingsModalProps {
  children: React.ReactNode; // To wrap the trigger button
}

function SettingsModal({ children }: SettingsModalProps) {
  // Theme state
  const { baseTheme, mode, setBaseTheme, setMode } = useTheme();

  // AI Settings State - Use local types
  const [aiSettings, setAiSettings] = useState<LocalAIProviderSettings | null>(
    null,
  );
  const [selectedProvider, setSelectedProvider] =
    useState<AIProvider>("openai");
  const [isLoadingAISettings, setIsLoadingAISettings] = useState(true);

  // Fetch AI settings on mount
  useEffect(() => {
    async function loadAISettings() {
      try {
        setIsLoadingAISettings(true);
        const settingsFromBackend: services.AIProviderSettings =
          await GetAIProviderSettings();

        // Map backend data to local state structure
        const localSettings: LocalAIProviderSettings = {
          provider: (settingsFromBackend.provider as AIProvider) || "openai",
          openai: { ...settingsFromBackend.openai },
          anthropic: { ...settingsFromBackend.anthropic },
          openrouter: { ...settingsFromBackend.openrouter },
        };

        setAiSettings(localSettings);
        setSelectedProvider(localSettings.provider ?? "openai");
      } catch (error) {
        console.error("Failed to load AI settings:", error);
        // Initialize with empty local settings on error
        setAiSettings({
          provider: "openai",
          openai: { apiKey: "", baseURL: "" },
          anthropic: { apiKey: "", baseURL: "" },
          openrouter: { apiKey: "" },
        });
      } finally {
        setIsLoadingAISettings(false);
      }
    }
    loadAISettings();
  }, []);

  // Handlers for Theme
  const handleBaseThemeChange = (newBaseTheme: string) => {
    setBaseTheme(newBaseTheme);
  };

  const handleModeChange = (newMode: ThemeMode) => {
    setMode(newMode);
  };

  // Handler for individual AI field changes (API Key, Base URL)
  // Only updates local state, does not save immediately.
  const handleAISettingChange = <K extends keyof LocalAIProviderSettings>(
    provider: K,
    field: keyof NonNullable<LocalAIProviderSettings[K]>,
    value: string,
  ) => {
    setAiSettings((prevSettings) => {
      if (!prevSettings) return null;

      const updatedSettings: LocalAIProviderSettings = {
        provider: prevSettings.provider,
        ...prevSettings,
        [provider]: {
          ...((prevSettings[provider] as any) ?? {}),
          [field]: value,
        },
      };

      return updatedSettings;
    });
  };

  // Handler for changing the selected AI provider
  const handleProviderSelectionChange = (newProvider: AIProvider) => {
    setSelectedProvider(newProvider);

    setAiSettings((prevSettings) => {
      if (!prevSettings) {
        // Should ideally not happen if initialized correctly
        return { provider: newProvider };
      }

      const updatedSettings: LocalAIProviderSettings = {
        ...prevSettings,
        provider: newProvider,
      };

      // Now save the entire updated settings object to the backend
      SaveAIProviderSettings(
        updatedSettings as services.AIProviderSettings,
      ).catch((err) => {
        console.error("Failed to save AI settings on provider change:", err);
        // TODO: Add user feedback (e.g., toast notification)
      });

      return updatedSettings;
    });
  };

  const renderAIProviderFields = () => {
    if (isLoadingAISettings || !aiSettings) {
      return <p>Loading AI settings...</p>; // Or a spinner
    }

    switch (selectedProvider) {
      case "openai":
        return (
          <>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="openai-apikey" className="text-right">
                API Key
              </Label>
              <Input
                id="openai-apikey"
                type="password"
                value={aiSettings.openai?.apiKey ?? ""}
                onChange={(e) =>
                  handleAISettingChange("openai", "apiKey", e.target.value)
                }
                className="col-span-3"
                placeholder="sk-..."
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="openai-baseurl" className="text-right">
                Base URL
              </Label>
              <Input
                id="openai-baseurl"
                value={aiSettings.openai?.baseURL ?? ""}
                onChange={(e) =>
                  handleAISettingChange("openai", "baseURL", e.target.value)
                }
                className="col-span-3"
                placeholder="Optional, default: https://api.openai.com/v1"
              />
            </div>
          </>
        );
      case "anthropic":
        return (
          <>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="anthropic-apikey" className="text-right">
                API Key
              </Label>
              <Input
                id="anthropic-apikey"
                type="password"
                value={aiSettings.anthropic?.apiKey ?? ""}
                onChange={(e) =>
                  handleAISettingChange("anthropic", "apiKey", e.target.value)
                }
                className="col-span-3"
                placeholder="sk-ant-..."
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="anthropic-baseurl" className="text-right">
                Base URL
              </Label>
              <Input
                id="anthropic-baseurl"
                value={aiSettings.anthropic?.baseURL ?? ""}
                onChange={(e) =>
                  handleAISettingChange("anthropic", "baseURL", e.target.value)
                }
                className="col-span-3"
                placeholder="Optional, default: https://api.anthropic.com/v1"
              />
            </div>
          </>
        );
      case "openrouter":
        return (
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="openrouter-apikey" className="text-right">
              API Key
            </Label>
            <Input
              id="openrouter-apikey"
              type="password"
              value={aiSettings.openrouter?.apiKey ?? ""}
              onChange={(e) =>
                handleAISettingChange("openrouter", "apiKey", e.target.value)
              }
              className="col-span-3"
              placeholder="sk-or-..."
            />
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <Dialog>
      <DialogTrigger asChild>
        <TooltipTrigger asChild>{children}</TooltipTrigger>
      </DialogTrigger>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>Preferences</DialogTitle>
        </DialogHeader>

        <fieldset className="grid grid-cols-4 items-center gap-4">
          <Label htmlFor="theme-mode" className="text-right col-span-1">
            Mode
          </Label>
          <RadioGroup
            value={mode}
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

        <fieldset className="grid grid-cols-4 items-center gap-4">
          <Label htmlFor="base-theme" className="text-right">
            Theme
          </Label>
          <Select value={baseTheme} onValueChange={handleBaseThemeChange}>
            <SelectTrigger
              className="col-span-3  shadow-none font-medium"
              id="base-theme"
            >
              <SelectValue placeholder="Select a theme" />
            </SelectTrigger>
            <SelectContent>
              {availableThemes.map((themeName) => (
                <SelectItem key={themeName} value={themeName}>
                  {themeName.split("-").map(capitalize).join(" ")}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </fieldset>

        <fieldset className="grid grid-cols-4 items-center gap-4">
          <Label htmlFor="ai-provider" className="text-right col-span-1">
            Provider
          </Label>
          <RadioGroup
            value={selectedProvider}
            onValueChange={(value) =>
              handleProviderSelectionChange(value as AIProvider)
            }
            className="col-span-3 flex space-x-2"
            id="ai-provider"
          >
            {aiProviders.map((provider) => (
              <div key={provider} className="flex items-center space-x-2">
                <RadioGroupItem value={provider} id={`r-${provider}`} />
                <Label htmlFor={`r-${provider}`}>{capitalize(provider)}</Label>
              </div>
            ))}
          </RadioGroup>
        </fieldset>

        {renderAIProviderFields()}

        <DialogFooter className="mt-4 pt-4">
          <DialogClose asChild>
            <Button type="button" variant="secondary">
              Close
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default memo(SettingsModal);
