import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { dicts, tFor } from "../../core/i18n";
import type { Lang, TFunc } from "../../core/i18n";

export * from "../../core/i18n";

const STORAGE_KEY = "stocker-lang";

type I18nCtxValue = { lang: Lang; setLang: (l: Lang) => void; t: TFunc };

/* Default context = English, so components rendered without the provider
   (e.g. in unit tests) still work. */
const I18nCtx = createContext<I18nCtxValue>({ lang: "en", setLang: () => {}, t: tFor("en") });

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() =>
    localStorage.getItem(STORAGE_KEY) === "zh" ? "zh" : "en");

  useEffect(() => {
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
    document.title = dicts[lang]["app.title"];
  }, [lang]);

  const setLang = (l: Lang) => {
    localStorage.setItem(STORAGE_KEY, l);
    setLangState(l);
  };

  return <I18nCtx.Provider value={{ lang, setLang, t: tFor(lang) }}>{children}</I18nCtx.Provider>;
}

export const useT = () => useContext(I18nCtx);

export function LangSwitch() {
  const { lang, setLang } = useT();
  return (
    <div className="lang-switch">
      <button className={lang === "zh" ? "on" : ""} onClick={() => setLang("zh")}>中文</button>
      <button className={lang === "en" ? "on" : ""} onClick={() => setLang("en")}>EN</button>
    </div>
  );
}
