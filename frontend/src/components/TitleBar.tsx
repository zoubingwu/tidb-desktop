import { Loader } from "lucide-react";
import React, { memo } from "react";
import {
  WindowIsMaximised,
  WindowMaximise,
  WindowUnmaximise,
} from "wailsjs/runtime";

function TitleBar({ title, loading }: { title: string; loading?: boolean }) {
  const onDoubleClick = async () => {
    if (await WindowIsMaximised()) {
      WindowUnmaximise();
    } else {
      WindowMaximise();
    }
  };
  return (
    <div
      onDoubleClick={onDoubleClick}
      className="h-[28px] bg-[var(--card)] text-[var(--card-foreground)] flex justify-center items-center select-none text-xs border-b border-[var(--muted)] flex-shrink-0"
      style={{ "--wails-draggable": "drag" } as React.CSSProperties}
    >
      <div className="flex items-center gap-2">
        <span className="font-medium">{title}</span>
        {loading && <Loader className="size-3 animate-spin" />}
      </div>
    </div>
  );
}

export default memo(TitleBar);
