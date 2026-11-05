import eslint from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: [
      "**/coverage/**",
      "**/dist/**",
      "**/node_modules/**",
      "apps/desktop/frontend/bindings/**",
    ],
  },
  eslint.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
    },
  },
  {
    files: ["packages/ui/src/components/**/*.{ts,tsx}"],
    rules: {
      "react-refresh/only-export-components": "off",
    },
  },
  {
    files: ["packages/sdk/src/client.ts", "packages/sdk/src/contracts/management-client.ts"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [{
          regex: "(?:^|/)local-owner(?:\\.[jt]s)?$",
          message: "The common management contract and HTTP adapter must not depend on local owner capabilities.",
        }],
      }],
    },
  },
  {
    files: ["packages/ui/src/**/*.{ts,tsx}"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [{
          regex: "^@mender/(?:app|sdk)(?:/|$)|(?:^|/)(?:app|sdk)/src(?:/|$)",
          message: "UI primitives must not depend on product features or management contracts.",
        }],
      }],
    },
  },
  {
    files: ["packages/app/src/shared/**/*.{ts,tsx}"],
    ignores: ["**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [{
          regex: "(?:^|/)(?:domains|shell)(?:/|$)|(?:^|/)mender-app(?:\\.[jt]sx?)?$",
          message: "Shared code must not depend on features or app composition.",
        }],
      }],
    },
  },
  {
    files: ["packages/app/src/**/*.{ts,tsx}"],
    ignores: [
      "packages/app/src/domains/tools/**",
      "packages/app/src/shared/**",
      "**/*.test.{ts,tsx}",
    ],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [{
          regex: "(?:^|/)tools/(?!index(?:\\.[jt]sx?)?$)",
          message: "Use the public Tools entry point instead of an internal module.",
        }],
      }],
    },
  },
);
