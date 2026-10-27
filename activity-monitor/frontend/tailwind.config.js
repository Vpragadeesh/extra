/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Fira Sans"', 'Inter', 'ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'sans-serif'],
        mono: ['"Fira Code"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      colors: {
        // Semantic aliases over the default palette; classic slate/sky/emerald… stay available.
        ink: {
          950: '#090E1B', // page base
          900: '#0B1120', // raised surface
          800: '#111A2C', // tooltips / deepest panels
          700: '#16203A', // hover surface
        },
        accent: {
          DEFAULT: '#38BDF8', // sky-400
          soft: '#7DD3FC',    // sky-300, chart / data color
        },
        ok: '#22C55E',
      },
      container: { center: true },
      keyframes: {
        shimmer: {
          '0%': { backgroundPosition: '-400px 0' },
          '100%': { backgroundPosition: '400px 0' },
        },
      },
      animation: {
        shimmer: 'shimmer 1.4s linear infinite',
      },
    },
  },
  plugins: [],
};
