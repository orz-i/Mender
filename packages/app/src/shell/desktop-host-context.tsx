/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useMemo, type ReactNode } from "react";

export interface DesktopWindowControls {
  minimise?: (() => Promise<void>) | undefined;
  toggleMaximise?: (() => Promise<void>) | undefined;
  close?: (() => Promise<void>) | undefined;
}

export interface DesktopHostContextValue {
  revealLocalPath?: ((path: string) => Promise<void>) | undefined;
  windowControls?: DesktopWindowControls | undefined;
  isDesktop: boolean;
}

const DesktopHostContext = createContext<DesktopHostContextValue>({
  isDesktop: false,
});

export function DesktopHostProvider({
  children,
  revealLocalPath,
  windowControls,
  isDesktop,
}: {
  children: ReactNode;
  revealLocalPath?: ((path: string) => Promise<void>) | undefined;
  windowControls?: DesktopWindowControls | undefined;
  isDesktop: boolean;
}) {
  const value = useMemo<DesktopHostContextValue>(
    () => ({
      revealLocalPath,
      windowControls,
      isDesktop,
    }),
    [revealLocalPath, windowControls, isDesktop],
  );

  return (
    <DesktopHostContext.Provider value={value}>
      {children}
    </DesktopHostContext.Provider>
  );
}

export function useDesktopHost(): DesktopHostContextValue {
  return useContext(DesktopHostContext);
}
