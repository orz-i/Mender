import { describe, expect, it } from "vitest";
import {
  comparableWorkspacePath,
  defaultWorkspacePlaceholder,
  isAbsoluteWorkspacePath,
} from "./workspace-paths";

describe("isAbsoluteWorkspacePath", () => {
  it("recognizes POSIX absolute paths", () => {
    expect(isAbsoluteWorkspacePath("/")).toBe(true);
    expect(isAbsoluteWorkspacePath("/Users/example/project")).toBe(true);
    expect(isAbsoluteWorkspacePath("  /var/log/mender  ")).toBe(true);
  });

  it("recognizes Windows drive-letter absolute paths with forward or backward slashes", () => {
    expect(isAbsoluteWorkspacePath("D:\\Mender")).toBe(true);
    expect(isAbsoluteWorkspacePath("C:\\Users\\mini\\Projects\\Mender")).toBe(true);
    expect(isAbsoluteWorkspacePath("c:/projects/test")).toBe(true);
    expect(isAbsoluteWorkspacePath("  E:\\code\\mender  ")).toBe(true);
  });

  it("recognizes Windows UNC paths", () => {
    expect(isAbsoluteWorkspacePath("\\\\server\\share\\project")).toBe(true);
    expect(isAbsoluteWorkspacePath("//server/share/project")).toBe(true);
  });

  it("rejects relative paths and invalid prefixes", () => {
    expect(isAbsoluteWorkspacePath("")).toBe(false);
    expect(isAbsoluteWorkspacePath("   ")).toBe(false);
    expect(isAbsoluteWorkspacePath("relative/path")).toBe(false);
    expect(isAbsoluteWorkspacePath("./local/path")).toBe(false);
    expect(isAbsoluteWorkspacePath("../parent/path")).toBe(false);
    expect(isAbsoluteWorkspacePath("C:")).toBe(false);
    expect(isAbsoluteWorkspacePath("C:relative\\path")).toBe(false);
  });
});

describe("comparableWorkspacePath", () => {
  it("normalizes POSIX paths", () => {
    expect(comparableWorkspacePath("/Users/foo/bar/")).toBe("/Users/foo/bar");
    expect(comparableWorkspacePath("/Users/foo/bar")).toBe("/Users/foo/bar");
  });

  it("normalizes Windows paths and folds case for comparison", () => {
    expect(comparableWorkspacePath("D:\\Mender")).toBe("d:/mender");
    expect(comparableWorkspacePath("d:/Mender/")).toBe("d:/mender");
    expect(comparableWorkspacePath("D:\\Mender\\")).toBe(
      comparableWorkspacePath("d:/mender"),
    );
  });
});

describe("defaultWorkspacePlaceholder", () => {
  it("returns a placeholder string", () => {
    const placeholder = defaultWorkspacePlaceholder();
    expect(typeof placeholder).toBe("string");
    expect(placeholder.length).toBeGreaterThan(0);
  });
});
