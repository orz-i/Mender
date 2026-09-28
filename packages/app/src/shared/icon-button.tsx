import type { ComponentProps, ReactNode } from "react";

import { Button } from "@mender/ui/components/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@mender/ui/components/tooltip";

type ButtonProps = ComponentProps<typeof Button>;

type IconButtonProps = Omit<ButtonProps, "aria-label" | "children" | "size"> & {
  label: string;
  children: ReactNode;
  size?: "icon" | "icon-xs" | "icon-sm" | "icon-lg";
};

export function IconButton({
  label,
  children,
  size = "icon-sm",
  disabled,
  ...props
}: IconButtonProps) {
  if (disabled) {
    return (
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button
            {...props}
            aria-label={label}
            disabled
            size={size}
          >
            {children}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            {...props}
            aria-label={label}
            size={size}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
