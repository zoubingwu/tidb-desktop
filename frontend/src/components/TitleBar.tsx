import React, { memo } from "react";
import {
  WindowMaximise,
  WindowUnmaximise,
  WindowIsMaximised,
} from "wailsjs/runtime";

function TitleBar({ title }: { title: string }) {
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
      <span className="font-medium">{title}</span>
    </div>
  );
}

export default memo(TitleBar);
