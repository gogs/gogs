import { useEffect } from "react";

import { FormError } from "@/lib/queries/request";

// useFocusFirstError moves the focus to the first field, in display order,
// that the server rejected. It waits for the render that shows the error,
// since a form does not take the focus while it is disabled for submitting.
// A field has its form key as its ID, unless ids names another. Pass both as
// constants.
export function useFocusFirstError(
  error: Error | null,
  order: readonly string[],
  ids: Readonly<Record<string, string>> = FORM_KEYS_AS_IDS,
) {
  useEffect(() => {
    const fields = error instanceof FormError ? error.fields : {};
    const first = order.find((key) => key in fields);
    if (first) document.getElementById(ids[first] ?? first)?.focus();
  }, [error, order, ids]);
}

const FORM_KEYS_AS_IDS = {};
