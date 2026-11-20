import type { ErrorComponentProps } from "@tanstack/react-router";
import type { ReactNode } from "react";

import { NotFoundTerminal } from "@/pages/NotFound";
import { ServerErrorTerminal } from "@/pages/ServerError";

function Problem({ children }: { children: ReactNode }) {
  return <div className="flex justify-center py-4 sm:py-10">{children}</div>;
}

// AdminNotFound and AdminError stand in for a page of the admin console that
// is not found or failed to load. The console names the problem in the title
// and the breadcrumb.
export function AdminNotFound() {
  return (
    <Problem>
      <NotFoundTerminal />
    </Problem>
  );
}

export function AdminError({ error }: ErrorComponentProps) {
  return (
    <Problem>
      <ServerErrorTerminal error={error} />
    </Problem>
  );
}
