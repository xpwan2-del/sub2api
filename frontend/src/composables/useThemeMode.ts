import { onMounted, ref } from 'vue'

export function useThemeMode() {
  const isDark = ref(document.documentElement.classList.contains('dark'))

  function applyTheme(nextDark: boolean) {
    isDark.value = nextDark
    document.documentElement.classList.toggle('dark', nextDark)
    localStorage.setItem('theme', nextDark ? 'dark' : 'light')
  }

  function toggleTheme() {
    applyTheme(!isDark.value)
  }

  function initTheme() {
    const savedTheme = localStorage.getItem('theme')
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    applyTheme(savedTheme === 'dark' || (!savedTheme && prefersDark))
  }

  onMounted(initTheme)

  return {
    isDark,
    toggleTheme
  }
}
