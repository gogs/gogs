import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";

// RelativeTime shows how long ago a time was, with the full date in a tooltip
// that opens on hover, on keyboard focus, and on a tap.
export function RelativeTime({ time }: { time: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const isTouchRef = useRef(false);
  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        {/* The tooltip ignores touch and closes on a press, so a tap opens it
            here instead. A tap anywhere else closes it. */}
        <time
          dateTime={time}
          tabIndex={0}
          className="rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--color-ring)"
          onPointerDown={(event) => {
            isTouchRef.current = event.pointerType === "touch";
            if (isTouchRef.current) event.preventDefault();
          }}
          onClick={(event) => {
            if (!isTouchRef.current) return;
            event.preventDefault();
            setOpen(true);
          }}
        >
          {formatRelativeTime(t, time)}
        </time>
      </TooltipTrigger>
      <TooltipContent>{formatAbsoluteTime(time)}</TooltipContent>
    </Tooltip>
  );
}
