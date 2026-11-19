import { useEffect } from "react";

import { FormError } from "@/lib/queries/request";

// useFocusFirstError moves the focus to the first field, in display order,
// that the server rejected. It waits for the render that shows the error,
// since a form does not take the focus while it is disabled for submitting.
// Pass the IDs of the fields as a constant.
export function useFocusFirstError(error: Error | null, order: readonly string[]) {
  useEffect(() => {
    const fields = error instanceof FormError ? error.fields : {};
    const first = order.find((id) => id in fields);
    if (first) document.getElementById(first)?.focus();
  }, [error, order]);
}
