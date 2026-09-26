import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { MenderApp } from "./mender-app";

describe("MenderApp", () => {
  it("renders the foundation shell", () => {
    render(
      <MemoryRouter>
        <MenderApp />
      </MemoryRouter>,
    );

    expect(
      screen.getByRole("heading", {
        name: "Tool runtime and gateway for LLMs and agents",
      }),
    ).toBeTruthy();
  });
});
