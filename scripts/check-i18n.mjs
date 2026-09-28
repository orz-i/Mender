import fs from "node:fs";
import path from "node:path";

import ts from "typescript";

const sourceRoot = path.resolve("packages/app/src");
const zhPath = path.join(sourceRoot, "i18n/zh-cn.ts");

const translatablePropNames = new Set([
  "description",
  "emptyLabel",
  "label",
  "submitLabel",
  "title",
]);

const dynamicMessageKeys = new Set([
  "allow",
  "any",
  "deny",
  "direct",
  "disabled",
  "dynamic",
  "enabled",
  "environment",
  "good",
  "hybrid",
  "known",
  "native",
  "neutral",
  "require approval",
  "unknown",
  "warning",
]);

const allowedRawJsxText = new Set([
  "Mender",
  "get /users/",
  "mender serve mcp --profile … --stdio",
]);

const zhSource = fs.readFileSync(zhPath, "utf8");
const zhSourceFile = ts.createSourceFile(
  zhPath,
  zhSource,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TS,
);
const zhMessages = new Map();

function collectZhMessages(node) {
  if (
    ts.isPropertyAssignment(node) &&
    ts.isStringLiteralLike(node.name) &&
    ts.isStringLiteralLike(node.initializer)
  ) {
    zhMessages.set(node.name.text, node.initializer.text);
  }
  ts.forEachChild(node, collectZhMessages);
}
collectZhMessages(zhSourceFile);

const requiredMessages = new Set(dynamicMessageKeys);
const rawJsxText = [];

for (const file of sourceFiles(sourceRoot)) {
  const source = fs.readFileSync(file, "utf8");
  const sourceFile = ts.createSourceFile(
    file,
    source,
    ts.ScriptTarget.Latest,
    true,
    file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );

  const visit = (node) => {
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "t" &&
      node.arguments[0] !== undefined
    ) {
      collectRenderedMessageKeys(node.arguments[0], requiredMessages);
    }

    if (
      ts.isJsxAttribute(node) &&
      translatablePropNames.has(node.name.getText(sourceFile)) &&
      node.initializer !== undefined &&
      ts.isStringLiteral(node.initializer)
    ) {
      requiredMessages.add(node.initializer.text);
    }

    if (
      ts.isPropertyAssignment(node) &&
      node.name.getText(sourceFile) === "label" &&
      ts.isStringLiteralLike(node.initializer)
    ) {
      requiredMessages.add(node.initializer.text);
    }

    if (ts.isJsxText(node)) {
      const text = node.getText(sourceFile).replace(/\s+/g, " ").trim();
      if (/[A-Za-z]/.test(text) && !allowedRawJsxText.has(text)) {
        rawJsxText.push(
          `${path.relative(process.cwd(), file)}:${
            sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile))
              .line + 1
          }: ${text}`,
        );
      }
    }

    ts.forEachChild(node, visit);
  };

  visit(sourceFile);
}

const missing = [...requiredMessages]
  .filter((message) => !zhMessages.has(message))
  .sort();
const blank = [...zhMessages]
  .filter(([, value]) => value.trim() === "")
  .map(([message]) => message)
  .sort();

if (missing.length || blank.length || rawJsxText.length) {
  if (missing.length) {
    console.error("Missing zh-CN translations:");
    for (const message of missing) console.error(`  - ${message}`);
  }
  if (blank.length) {
    console.error("Blank zh-CN translations:");
    for (const message of blank) console.error(`  - ${message}`);
  }
  if (rawJsxText.length) {
    console.error("Unlocalized raw JSX text:");
    for (const message of rawJsxText) console.error(`  - ${message}`);
  }
  process.exitCode = 1;
} else {
  console.log(
    `i18n coverage OK: ${requiredMessages.size} required messages, ${zhMessages.size} zh-CN translations`,
  );
}

function collectRenderedMessageKeys(node, messages) {
  if (ts.isStringLiteralLike(node)) {
    messages.add(node.text);
    return;
  }

  if (ts.isParenthesizedExpression(node)) {
    collectRenderedMessageKeys(node.expression, messages);
    return;
  }

  if (ts.isConditionalExpression(node)) {
    collectRenderedMessageKeys(node.whenTrue, messages);
    collectRenderedMessageKeys(node.whenFalse, messages);
    return;
  }

  if (
    ts.isBinaryExpression(node) &&
    (node.operatorToken.kind === ts.SyntaxKind.BarBarToken ||
      node.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken)
  ) {
    collectRenderedMessageKeys(node.left, messages);
    collectRenderedMessageKeys(node.right, messages);
  }
}

function sourceFiles(root) {
  const files = [];

  const walk = (directory) => {
    for (const name of fs.readdirSync(directory)) {
      const file = path.join(directory, name);
      const stat = fs.statSync(file);
      if (stat.isDirectory()) {
        if (name !== "i18n") walk(file);
        continue;
      }

      if (
        (name.endsWith(".ts") || name.endsWith(".tsx")) &&
        !name.includes(".test.")
      ) {
        files.push(file);
      }
    }
  };

  walk(root);
  files.push(path.join(root, "i18n/i18n.ts"));
  return files;
}
