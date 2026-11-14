import { useEffect, useState } from "react";

// useDebounced returns the value once it stopped changing for a moment, so
// that typing does not ask the server at every key.
export function useDebounced<T>(value: T, delay = 150) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}
