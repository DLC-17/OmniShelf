export type Theme =
  | 'dark-espresso'
  | 'midnight-oled'
  | 'cream-paper'
  | 'dracula'
  | 'obsidian'
  | 'e-ink'
  | 'nord'
  | 'solarized-dark'
  | 'catppuccin'

export interface ThemeOption {
  id: Theme
  name: string
  description: string
  colors: {
    bg: string
    surface: string
    accent: string
    text: string
    border: string
  }
}

export const THEMES: ThemeOption[] = [
  {
    id: 'dark-espresso',
    name: 'Dark Espresso',
    description: 'Warm espresso tones with rich cream highlights (Default)',
    colors: {
      bg: '#21120f',
      surface: '#2e1815',
      accent: '#d9b095',
      text: '#ffe0b5',
      border: '#4c322b',
    },
  },
  {
    id: 'midnight-oled',
    name: 'Midnight OLED',
    description: 'True pitch black background for OLED screens & high contrast',
    colors: {
      bg: '#000000',
      surface: '#0a0a0c',
      accent: '#38bdf8',
      text: '#f4f4f5',
      border: '#27272a',
    },
  },
  {
    id: 'cream-paper',
    name: 'Cream Paper',
    description: 'Light, warm, vintage paper aesthetic with crisp typography',
    colors: {
      bg: '#fbf5ec',
      surface: '#ffffff',
      accent: '#8a6552',
      text: '#462521',
      border: '#ecdfcb',
    },
  },
  {
    id: 'dracula',
    name: 'Dracula',
    description: 'Classic Dracula palette — deep purples with neon accents',
    colors: {
      bg: '#282a36',
      surface: '#343746',
      accent: '#bd93f9',
      text: '#f8f8f2',
      border: '#44475a',
    },
  },
  {
    id: 'obsidian',
    name: 'Obsidian',
    description: 'Inspired by volcanic glass — inky blacks with warm amber',
    colors: {
      bg: '#1a1a2e',
      surface: '#23234a',
      accent: '#e2b24a',
      text: '#eaeaea',
      border: '#33335a',
    },
  },
  {
    id: 'e-ink',
    name: 'E-Ink Monochrome',
    description: 'Pure high-contrast black & white optimized for e-readers & sunlight',
    colors: {
      bg: '#ffffff',
      surface: '#ffffff',
      accent: '#000000',
      text: '#000000',
      border: '#000000',
    },
  },
  {
    id: 'nord',
    name: 'Nord',
    description: 'Arctic, north-bluish palette with clean frosty aesthetics',
    colors: {
      bg: '#2e3440',
      surface: '#3b4252',
      accent: '#88c0d0',
      text: '#eceff4',
      border: '#4c566a',
    },
  },
  {
    id: 'solarized-dark',
    name: 'Solarized Dark',
    description: 'Precision colors engineered for low eye-strain & dark environments',
    colors: {
      bg: '#002b36',
      surface: '#073642',
      accent: '#268bd2',
      text: '#93a1a1',
      border: '#586e75',
    },
  },
  {
    id: 'catppuccin',
    name: 'Catppuccin Macchiato',
    description: 'Soothing warm pastel palette with rich medium-contrast undertones',
    colors: {
      bg: '#24273a',
      surface: '#1e2030',
      accent: '#c6a0f6',
      text: '#cad3f5',
      border: '#494d64',
    },
  },
]

export const THEME_STORAGE_KEY = 'omnishelf_theme'

export function getStoredTheme(): Theme {
  try {
    const saved = localStorage.getItem(THEME_STORAGE_KEY) as Theme | null
    if (saved && THEMES.some((t) => t.id === saved)) {
      return saved
    }
  } catch {
    // localStorage may not be accessible in some environments
  }
  return 'dark-espresso'
}

export function applyTheme(theme: Theme): void {
  try {
    document.documentElement.dataset.theme = theme
    localStorage.setItem(THEME_STORAGE_KEY, theme)
  } catch {
    // ignore
  }
}

export function initTheme(): Theme {
  const theme = getStoredTheme()
  applyTheme(theme)
  return theme
}
