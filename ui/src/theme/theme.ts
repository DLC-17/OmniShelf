// theme.ts – defines theme identifiers matching the backend and CSS `[data-theme]` selectors.
export type Theme = "dark-espresso" | "midnight-oled" | "cream-paper" | "dracula" | "obsidian";

export const themes: Record<Theme, string> = {
  "dark-espresso": "dark-espresso",
  "midnight-oled": "midnight-oled",
  "cream-paper": "cream-paper",
  "dracula": "dracula",
  "obsidian": "obsidian",
};
