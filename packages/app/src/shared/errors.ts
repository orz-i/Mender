import {
  MenderApiError,
  MenderClientInvariantError,
} from "@mender/sdk";

import { getLocale, t } from "../i18n/i18n";

function apiErrorMessage(code: string): string {
  switch (code) {
    case "conflict":
      return t("The request conflicts with the current state.");
    case "execution_error":
      return t("Tool execution failed.");
    case "http_error":
      return t("Mender API request failed.");
    case "internal_error":
      return t("Mender encountered an internal error.");
    case "not_found":
      return t("The requested item was not found.");
    case "provider_error":
      return t("The Provider request failed.");
    case "request_forbidden":
      return t("The request was rejected by local security policy.");
    case "validation_error":
      return t("The request could not be validated.");
    default:
      return t("Mender request failed.");
  }
}

export function errorMessage(reason: unknown): string {
  if (reason instanceof MenderClientInvariantError) {
    return getLocale() === "en"
      ? reason.message
      : t("Desktop returned data incompatible with this version of Mender.");
  }

  if (reason instanceof MenderApiError) {
    return getLocale() === "en" ? reason.message : apiErrorMessage(reason.code);
  }

  if (reason instanceof Error) {
    if (getLocale() === "en") {
      return reason.message;
    }

    const translated = t(reason.message);
    if (translated !== reason.message || /[\u3400-\u9fff]/u.test(reason.message)) {
      return translated;
    }
  }

  return t("Unexpected error");
}
