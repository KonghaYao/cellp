import type { ReactElement, ReactNode } from "react";
import { render, type RenderOptions } from "@testing-library/react";
import {
  MemoryRouter,
  Route,
  Routes,
  type MemoryRouterProps,
} from "react-router-dom";

type RouterOptions = {
  initialEntries?: MemoryRouterProps["initialEntries"];
  routes?: ReactNode;
};

export function renderWithRouter(
  ui: ReactElement,
  { initialEntries = ["/"], routes }: RouterOptions = {},
  options?: Omit<RenderOptions, "wrapper">,
) {
  function Wrapper({ children }: { children: ReactNode }) {
    if (routes) {
      return (
        <MemoryRouter initialEntries={initialEntries}>{routes}</MemoryRouter>
      );
    }
    return (
      <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>
    );
  }

  return render(ui, { wrapper: Wrapper, ...options });
}

export { Route, Routes, MemoryRouter };
