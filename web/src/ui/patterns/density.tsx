import { createContext, type ReactNode, useContext } from "react";

// Only skeletons select management or viewing density; sections derive spacing and typography from it.

export type Density = "management" | "viewing";

const DensityContext = createContext<Density>("management");

/** For skeletons only, not screens. */
export function DensityProvider({
  density,
  children,
}: {
  density: Density;
  children: ReactNode;
}) {
  return <DensityContext value={density}>{children}</DensityContext>;
}

export function useDensity(): Density {
  return useContext(DensityContext);
}
