import React, { memo } from "react";

function TitleBar({ title }: { title: string }) {
  return (
    <div
      className="h-[28px] bg-[var(--card)] text-[var(--card-foreground)] flex justify-center items-center select-none text-xs border-b border-[var(--border)] flex-shrink-0"
      style={{ "--wails-draggable": "drag" } as React.CSSProperties}
    >
      <span className="font-medium">{title}</span>
    </div>
  );
}

export default memo(TitleBar);
