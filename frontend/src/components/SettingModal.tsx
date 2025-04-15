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
import {
  useTheme,
  availableThemes,
  type ThemeMode,
} from "@/components/ThemeProvider";

interface SettingsModalProps {
  children: React.ReactNode; // To wrap the trigger button
}

export function SettingsModal({ children }: SettingsModalProps) {
  const { baseTheme, mode, setBaseTheme, setMode } = useTheme();

  const handleBaseThemeChange = (newBaseTheme: string) => {
    setBaseTheme(newBaseTheme);
  };

  const handleModeChange = (newMode: ThemeMode) => {
    setMode(newMode);
  };

  return (
    <Dialog>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
        </DialogHeader>
        <div className="grid gap-6 py-4">
          {/* Mode Selection */}
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

          {/* Base Theme Selection */}
          <fieldset className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="base-theme" className="text-right">
              Theme
            </Label>
            <Select value={baseTheme} onValueChange={handleBaseThemeChange}>
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
        </div>
        <DialogFooter>
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
