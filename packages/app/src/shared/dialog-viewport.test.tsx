import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@mender/ui/components/dialog";
import { Button } from "@mender/ui/components/button";

afterEach(cleanup);

it("constrains every ordinary Dialog popup to the visual viewport", () => {
  render(
    <Dialog open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Viewport guard</DialogTitle>
          <DialogDescription>Long content must never hide the whole popup.</DialogDescription>
        </DialogHeader>
        <div>{Array.from({ length: 65 }, (_, i) => <p key={i}>Information {i}</p>)}</div>
        <DialogFooter><Button>Finish</Button></DialogFooter>
      </DialogContent>
    </Dialog>,
  );
  const popup = screen.getByRole("dialog", { name: "Viewport guard" });
  expect(popup.className).toContain("max-h-[calc(100dvh-2rem)]");
  expect(popup.className).toContain("overflow-y-auto");
  expect(popup.className).toContain("overscroll-contain");
  expect(screen.getByRole("button", { name: "Finish" })).toBeTruthy();
});
