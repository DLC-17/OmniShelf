import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { Theme } from "./theme";

interface ThemeContextProps {
  theme: Theme;
  setTheme: (t: Theme) => void;
}

const ThemeContext = createContext<ThemeContextProps | undefined>(undefined);

const DEFAULT_THEME: Theme = "dark-espresso";

export const ThemeProvider = ({ children }: { children: ReactNode }) => {
  const [theme, setThemeState] = useState<Theme>(DEFAULT_THEME);

  // Load from server on mount
  useEffect(() => {
    fetch("/api/user/theme")
      .then((r) => {
        if (!r.ok) throw new Error("not ok");
        return r.json() as Promise<{ theme: Theme }>;
      })
      .then(({ theme: t }) => {
        setThemeState(t);
        applyThemeAttr(t);
      })
      .catch(() => {
        // fallback to localStorage or default
        const stored = (localStorage.getItem("omnishelf_theme") as Theme) || DEFAULT_THEME;
        setThemeState(stored);
        applyThemeAttr(stored);
      });
  }, []);

  const applyThemeAttr = (t: Theme) => {
    document.documentElement.dataset.theme = t;
  };

  const setTheme = (t: Theme) => {
    setThemeState(t);
    applyThemeAttr(t);
    localStorage.setItem("omnishelf_theme", t);
    // Persist to server (ignore errors)
    fetch("/api/user/theme", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ theme: t }),
    }).catch(() => {});
  };

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>{children}</ThemeContext.Provider>
  );
};

export const useTheme = (): ThemeContextProps => {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider");
  return ctx;
};
