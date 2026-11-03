/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter Variable', 'Inter', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
      },
      // Semantic design tokens — map onto the slate/blue palette already in use
      // so components can migrate to bg-surface / text-muted / accent over time.
      colors: {
        surface: {
          DEFAULT: '#020617', // slate-950 — app background
          raised: '#0f172a',  // slate-900 — cards / modals
          muted: '#1e293b',   // slate-800 — inset panels, list rows
        },
        accent: {
          DEFAULT: '#2563eb', // blue-600
          hover: '#3b82f6',   // blue-500
          soft: '#60a5fa',    // blue-400 — text/icons on dark
        },
        content: {
          DEFAULT: '#f1f5f9', // slate-100 — primary text
          muted: '#94a3b8',   // slate-400 — secondary text
          faint: '#64748b',   // slate-500 — tertiary / placeholders
        },
      },
    },
  },
  plugins: [],
}
