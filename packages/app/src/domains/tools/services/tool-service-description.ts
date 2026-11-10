import type { Provider } from "@mender/sdk";

// v3 Tool Services are independently configured Providers. Agent and native
// tools have their own typed sources in Catalog and never synthesize services.
export function toolServiceDescription(provider: Provider): string {
  return provider.description || "";
}
