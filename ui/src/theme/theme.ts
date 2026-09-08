// theme.ts – defines theme identifiers matching the backend and CSS `[data-theme]` selectors.
export type { Theme } from "../lib/theme";
import type { Theme } from "../lib/theme";

export const themes: Record<Theme, string> = {
  "dark-espresso": "dark-espresso",
  "midnight-oled": "midnight-oled",
  "cream-paper": "cream-paper",
  "dracula": "dracula",
  "obsidian": "obsidian",
  "e-ink": "e-ink",
  "nord": "nord",
  "solarized-dark": "solarized-dark",
  "catppuccin": "catppuccin",
};

