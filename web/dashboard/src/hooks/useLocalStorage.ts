import { useCallback, useEffect, useState } from "react";

export function useLocalStorage<T>(
  key: string,
  initialValue: T,
): [T, (value: T | ((prev: T) => T)) => void] {
  const readValue = useCallback((): T => {
    if (typeof window === "undefined") {
      return initialValue;
    }

    try {
      const item = window.localStorage.getItem(key);
      return item ? (JSON.parse(item) as T) : initialValue;
    } catch {
      return initialValue;
    }
  }, [initialValue, key]);

  const [storedValue, setStoredValue] = useState<T>(readValue);

  const setValue = useCallback(
    (value: T | ((prev: T) => T)) => {
      setStoredValue((prev) => {
        const next = value instanceof Function ? value(prev) : value;

        try {
          window.localStorage.setItem(key, JSON.stringify(next));
        } catch {
          // Ignore quota / private mode errors.
        }

        return next;
      });
    },
    [key],
  );

  useEffect(() => {
    setStoredValue(readValue());
  }, [readValue]);

  return [storedValue, setValue];
}
