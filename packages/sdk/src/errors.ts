export class MenderClientInvariantError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "MenderClientInvariantError";
  }
}
