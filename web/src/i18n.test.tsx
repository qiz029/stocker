import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { I18nProvider, LangSwitch, mediaName, tFor, useT } from "./i18n";

function Probe() {
  const { t } = useT();
  return <div>{t("auth.login")}</div>;
}

beforeEach(() => localStorage.clear());

describe("i18n", () => {
  it("defaults to English and switches to Chinese, persisting the choice", () => {
    render(<I18nProvider><LangSwitch /><Probe /></I18nProvider>);
    expect(screen.getByText("Log in")).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en");
    expect(document.title).toBe("Stocker · Blind-Box Market");

    fireEvent.click(screen.getByRole("button", { name: "中文" }));
    expect(screen.getByText("登录")).toBeInTheDocument();
    expect(localStorage.getItem("stocker-lang")).toBe("zh");
    expect(document.documentElement.lang).toBe("zh-CN");
    expect(document.title).toBe("Stocker · 盲盒行情台");

    fireEvent.click(screen.getByRole("button", { name: "EN" }));
    expect(screen.getByText("Log in")).toBeInTheDocument();
    expect(localStorage.getItem("stocker-lang")).toBe("en");
  });

  it("restores the persisted language on load", () => {
    localStorage.setItem("stocker-lang", "zh");
    render(<I18nProvider><Probe /></I18nProvider>);
    expect(screen.getByText("登录")).toBeInTheDocument();
  });

  it("interpolates params and translates media names with id fallback", () => {
    expect(tFor("en")("common.day", { day: 3 })).toBe("Day 3");
    expect(tFor("zh")("common.day", { day: 3 })).toBe("第 3 日");
    expect(mediaName("wire", tFor("en"))).toBe("Wire Service");
    expect(mediaName("wire", tFor("zh"))).toBe("通讯社");
    expect(mediaName("unknown-outlet", tFor("en"))).toBe("unknown-outlet");
  });
});
