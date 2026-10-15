export function preference(key: 'language' | 'theme' | 'kind'): string | null {
  const name = `agentup_ui_${key}=`
  const cookie = document.cookie.split(';').map(value => value.trim()).find(value => value.startsWith(name))
  return cookie ? cookie.slice(name.length) : localStorage.getItem(`agentup.${key}`)
}

export function remember(key: 'language' | 'theme' | 'kind', value: string) {
  localStorage.setItem(`agentup.${key}`, value)
  // Display preferences, unlike session credentials, can follow loopback port changes.
  document.cookie = `agentup_ui_${key}=${value}; Path=/; SameSite=Strict; Max-Age=31536000`
}
