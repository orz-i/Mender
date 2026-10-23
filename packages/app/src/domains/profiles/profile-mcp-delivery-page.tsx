import { ChevronLeftIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type { MenderClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { t } from "../../i18n/i18n";
import { ServersPage } from "../servers/servers-page";

/** The existing Runtime-managed MCP lifecycle, explicitly scoped to Profile. */
export function ProfileMCPDeliveryPage({
  client,
  copyText,
}: {
  client: MenderClient;
  copyText?: (value: string) => Promise<void>;
}) {
  const { slug } = useParams();
  const navigate = useNavigate();
  if (!slug) return null;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Button variant="outline" size="sm" onClick={() =>
          void navigate("/profiles/" + encodeURIComponent(slug), { state: { initialSection: "deliveries" } })}>
          <ChevronLeftIcon data-icon="inline-start" />
          {t("Back to profile")}
        </Button>
        <div className="min-w-0">
          <h1 className="text-base font-semibold">{t("Profile MCP delivery")}</h1>
          <p className="text-xs text-muted-foreground">
            {t("Manage this Profile's running loopback MCP servers and their linked Tunnels.")}
          </p>
        </div>
      </div>
      <ServersPage client={client} profileSlug={slug}
        {...(copyText === undefined ? {} : { copyText })} />
    </div>
  );
}
