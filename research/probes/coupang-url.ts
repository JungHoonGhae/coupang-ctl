function parsedHostname(value: string): string | null {
  try {
    return new URL(value).hostname;
  } catch {
    return null;
  }
}

export function isCoupangHostURL(value: string): boolean {
  const hostname = parsedHostname(value);
  return hostname === "coupang.com" || hostname?.endsWith(".coupang.com") === true;
}

export function isCoupangLoginURL(value: string): boolean {
  return parsedHostname(value) === "login.coupang.com";
}
