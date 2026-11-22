import * as React from "react";

import { cn } from "@/lib/utils";

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div className={cn("rounded-md bg-(--color-foreground)/10 motion-safe:animate-pulse", className)} {...props} />
  );
}

export { Skeleton };
