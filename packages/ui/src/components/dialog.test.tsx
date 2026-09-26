import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "./dialog";

describe("Dialog layering", () => {
  it("keeps the backdrop below the interactive popup", () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Layer test</DialogTitle>
        </DialogContent>
      </Dialog>,
    );

    expect(screen.getByText("Layer test")).toBeTruthy();

    const overlay = document.querySelector<HTMLElement>(
      '[data-slot="dialog-overlay"]',
    );
    const content = document.querySelector<HTMLElement>(
      '[data-slot="dialog-content"]',
    );

    expect(overlay?.classList.contains("z-40")).toBe(true);
    expect(overlay?.classList.contains("z-50")).toBe(false);
    expect(content?.classList.contains("z-50")).toBe(true);
    expect(content?.classList.contains("pointer-events-auto")).toBe(true);
    expect(content?.classList.contains("[&>*]:min-w-0")).toBe(true);
  });
});
