import {
  createContext,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

interface HeaderSlotContextValue {
  toolbar: ReactNode | null;
  setToolbar: (toolbar: ReactNode | null) => void;
}

const HeaderSlotContext = createContext<HeaderSlotContextValue | null>(null);

export function HeaderSlotProvider({ children }: { children: ReactNode }) {
  const [toolbar, setToolbar] = useState<ReactNode | null>(null);
  const value = useMemo(() => ({ toolbar, setToolbar }), [toolbar]);

  return (
    <HeaderSlotContext.Provider value={value}>{children}</HeaderSlotContext.Provider>
  );
}

export function useHeaderSlot() {
  const context = useContext(HeaderSlotContext);
  if (!context) {
    throw new Error("useHeaderSlot must be used within HeaderSlotProvider");
  }
  return context;
}
