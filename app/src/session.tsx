import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { getLocales } from "expo-localization";
import { api, setApiBase, User } from "@core/api";
import { Lang, TFunc, tFor } from "@core/i18n";
import { API_BASE } from "./config";
import { registerPushToken, unregisterPushToken } from "./notifications";

const LANG_KEY = "stocker.lang";

type Session = {
  user: User | null;
  loading: boolean;               // true while the boot GET /api/me is in flight
  lang: Lang;
  t: TFunc;
  setLang: (lang: Lang) => void;
  setUser: (user: User) => void;  // called after a successful login/register
  logout: () => Promise<void>;
};

const Ctx = createContext<Session | null>(null);

function deviceLang(): Lang {
  const tag = getLocales()[0]?.languageCode ?? "en";
  return tag === "zh" ? "zh" : "en";
}

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [user, setUserState] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [lang, setLangState] = useState<Lang>(deviceLang());

  useEffect(() => {
    setApiBase(API_BASE);
    void (async () => {
      try {
        const stored = await AsyncStorage.getItem(LANG_KEY);
        if (stored === "en" || stored === "zh") setLangState(stored);
      } catch { /* keep device default */ }
      try {
        const me = await api.get<User>("/api/me");
        setUserState(me);
      } catch { /* 401 → logged out */ }
      setLoading(false);
    })();
  }, []);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    void AsyncStorage.setItem(LANG_KEY, l).catch(() => undefined);
  }, []);

  const setUser = useCallback((u: User) => setUserState(u), []);

  // Register the device's Expo push token whenever a session is active
  // (boot with valid cookie, login, register). Best-effort; see
  // src/notifications.ts.
  const userID = user?.id;
  useEffect(() => {
    if (userID !== undefined) void registerPushToken();
  }, [userID]);

  const logout = useCallback(async () => {
    await unregisterPushToken();
    try { await api.post("/api/logout"); } catch { /* best effort */ }
    setUserState(null);
  }, []);

  const value = useMemo<Session>(
    () => ({ user, loading, lang, t: tFor(lang), setLang, setUser, logout }),
    [user, loading, lang, setLang, setUser, logout],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession(): Session {
  const s = useContext(Ctx);
  if (!s) throw new Error("useSession outside SessionProvider");
  return s;
}
