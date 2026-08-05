import Constants from "expo-constants";

/* API origin for the Go backend. Configured via app.json extra.apiBase.
   A physical device must use the Mac's LAN IP instead of localhost —
   see app/README.md. */
export const API_BASE: string =
  (Constants.expoConfig?.extra?.apiBase as string | undefined) ?? "http://localhost:8080";
