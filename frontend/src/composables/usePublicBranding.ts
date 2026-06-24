import { computed } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

export function usePublicBranding() {
  const appStore = useAppStore()

  const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'TOP-AI')
  const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '')
  const siteLogo = computed(() =>
    sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
      allowRelative: true,
      allowDataUrl: true
    })
  )
  const brandLogo = computed(() => siteLogo.value || '/top-ai-logo.png')
  const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
  const docLink = computed(() => docUrl.value || '/docs')
  const isExternalDocUrl = computed(() => /^https?:\/\//i.test(docLink.value))
  const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

  return {
    appStore,
    siteName,
    siteSubtitle,
    siteLogo,
    brandLogo,
    docUrl,
    docLink,
    isExternalDocUrl,
    homeContent
  }
}
