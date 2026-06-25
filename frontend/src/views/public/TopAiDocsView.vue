<template>
  <div class="topai-docs-page" :class="{ 'is-dark': isDark }">
    <header class="topai-docs-header">
      <router-link to="/home" class="topai-docs-brand">
        <span class="topai-docs-logo-wrap">
          <img :src="siteLogo || '/top-ai-logo.png'" :alt="siteName" class="topai-docs-logo" />
        </span>
        <span class="topai-docs-brand-copy">
          <span class="topai-docs-brand-name">{{ siteName }}</span>
          <span class="topai-docs-brand-subtitle">{{ siteSubtitle }}</span>
        </span>
      </router-link>

      <div class="topai-docs-actions">
        <LocaleSwitcher />
        <button
          class="topai-docs-icon-button"
          :title="isDark ? copy.switchToLight : copy.switchToDark"
          @click="toggleTheme"
        >
          <Icon v-if="isDark" name="sun" size="md" />
          <Icon v-else name="moon" size="md" />
        </button>
        <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="topai-docs-primary-link">
          <span v-if="isAuthenticated" class="topai-docs-user-mark">{{ userInitial }}</span>
          <span>{{ isAuthenticated ? copy.dashboard : copy.login }}</span>
          <Icon name="arrowRight" size="xs" />
        </router-link>
      </div>
    </header>

    <main class="topai-docs-main">
      <section class="topai-docs-hero">
        <p class="topai-docs-kicker">{{ copy.kicker }}</p>
        <h1>{{ copy.title }}</h1>
        <p>{{ copy.description }}</p>
        <div class="topai-docs-hero-actions">
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="topai-docs-cta">
            {{ isAuthenticated ? copy.openDashboard : copy.startNow }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
          <a href="#quick-start" class="topai-docs-secondary-link">{{ copy.readSteps }}</a>
        </div>
      </section>

      <nav class="topai-docs-nav" :aria-label="copy.pageNav">
        <a href="#quick-start">{{ copy.nav.quickStart }}</a>
        <a href="#daily-use">{{ copy.nav.dailyUse }}</a>
        <a href="#faq">{{ copy.nav.faq }}</a>
      </nav>

      <section id="quick-start" class="topai-docs-section">
        <div class="topai-docs-section-heading">
          <p>{{ copy.quickStart.kicker }}</p>
          <h2>{{ copy.quickStart.title }}</h2>
        </div>
        <div class="topai-docs-steps">
          <article v-for="(step, index) in copy.steps" :key="step.title" class="topai-docs-step">
            <div class="topai-docs-step-index">{{ index + 1 }}</div>
            <div class="topai-docs-step-icon">
              <Icon :name="step.icon" size="lg" />
            </div>
            <h3>{{ step.title }}</h3>
            <p>{{ step.body }}</p>
          </article>
        </div>
      </section>

      <section id="daily-use" class="topai-docs-section">
        <div class="topai-docs-section-heading">
          <p>{{ copy.daily.kicker }}</p>
          <h2>{{ copy.daily.title }}</h2>
        </div>
        <div class="topai-docs-use-grid">
          <article v-for="item in copy.daily.items" :key="item.title" class="topai-docs-use-card">
            <div class="topai-docs-use-icon">
              <Icon :name="item.icon" size="lg" />
            </div>
            <h3>{{ item.title }}</h3>
            <p>{{ item.body }}</p>
          </article>
        </div>
      </section>

      <section class="topai-docs-section topai-docs-note-section">
        <div>
          <p class="topai-docs-kicker">{{ copy.safeUse.kicker }}</p>
          <h2>{{ copy.safeUse.title }}</h2>
        </div>
        <ul>
          <li v-for="item in copy.safeUse.items" :key="item">
            <Icon name="checkCircle" size="sm" />
            <span>{{ item }}</span>
          </li>
        </ul>
      </section>

      <section id="faq" class="topai-docs-section">
        <div class="topai-docs-section-heading">
          <p>{{ copy.faq.kicker }}</p>
          <h2>{{ copy.faq.title }}</h2>
        </div>
        <div class="topai-docs-faq">
          <article v-for="item in copy.faq.items" :key="item.q">
            <h3>{{ item.q }}</h3>
            <p>{{ item.a }}</p>
          </article>
        </div>
      </section>
    </main>

    <footer class="topai-docs-footer">
      <span>© {{ currentYear }} {{ siteName }}. {{ copy.allRightsReserved }}</span>
      <router-link to="/home">{{ copy.backHome }}</router-link>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

type GuideIcon =
  | 'login'
  | 'creditCard'
  | 'key'
  | 'brain'
  | 'chartBar'
  | 'shield'
  | 'copy'
  | 'calculator'
  | 'chat'
  | 'questionCircle'

type GuideCopy = {
  kicker: string
  title: string
  description: string
  login: string
  dashboard: string
  openDashboard: string
  startNow: string
  readSteps: string
  switchToLight: string
  switchToDark: string
  pageNav: string
  allRightsReserved: string
  backHome: string
  nav: {
    quickStart: string
    dailyUse: string
    faq: string
  }
  quickStart: {
    kicker: string
    title: string
  }
  steps: Array<{
    icon: GuideIcon
    title: string
    body: string
  }>
  daily: {
    kicker: string
    title: string
    items: Array<{
      icon: GuideIcon
      title: string
      body: string
    }>
  }
  safeUse: {
    kicker: string
    title: string
    items: string[]
  }
  faq: {
    kicker: string
    title: string
    items: Array<{
      q: string
      a: string
    }>
  }
}

const { locale } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'TOP-AI')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Pioneers of AI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user?.email) return ''
  return user.email.charAt(0).toUpperCase()
})
const currentYear = new Date().getFullYear()
const isDark = ref(document.documentElement.classList.contains('dark'))

const copy = computed<GuideCopy>(() => {
  if (locale.value.startsWith('zh')) {
    return {
      kicker: `${siteName.value} 使用说明`,
      title: '把一个 API 密钥，当成你的 AI 工作入口。',
      description: '这里讲的是普通用户如何使用本站：登录账号、获取密钥、选择模型、充值套餐、查看用量。不会要求你理解代码、部署或后端结构。',
      login: '登录',
      dashboard: '控制台',
      openDashboard: '进入控制台',
      startNow: '开始使用',
      readSteps: '查看步骤',
      switchToLight: '切换到浅色模式',
      switchToDark: '切换到深色模式',
      pageNav: '文档导航',
      allRightsReserved: 'All rights reserved.',
      backHome: '返回首页',
      nav: {
        quickStart: '快速开始',
        dailyUse: '日常使用',
        faq: '常见问题',
      },
      quickStart: {
        kicker: '五步上手',
        title: '从登录到第一次调用',
      },
      steps: [
        {
          icon: 'login',
          title: '登录或注册账号',
          body: '进入登录页，使用管理员给你的账号登录；如果开放注册，也可以自行创建账号。',
        },
        {
          icon: 'creditCard',
          title: '确认余额或套餐',
          body: '进入控制台查看当前余额、套餐和可用模型。没有额度时，先按站点提供的充值或兑换方式补充额度。',
        },
        {
          icon: 'key',
          title: '创建 API 密钥',
          body: '在 API Keys 页面创建自己的密钥。密钥只显示一次，复制后请妥善保存，不要发给别人。',
        },
        {
          icon: 'brain',
          title: '选择模型与用途',
          body: '根据需要选择 GPT、Claude、Gemini、DeepSeek 等模型。写代码、长文档、推理、多模态场景可以选择不同模型。',
        },
        {
          icon: 'chartBar',
          title: '查看用量与扣费',
          body: '在用量页面查看请求记录、消耗和剩余额度。发现异常消耗时，及时停用密钥或联系管理员。',
        },
      ],
      daily: {
        kicker: '日常怎么用',
        title: '把平台当成统一 AI 网关',
        items: [
          {
            icon: 'copy',
            title: '复制密钥到你的工具',
            body: '把 API 密钥填到支持 OpenAI/Claude/Gemini 兼容配置的客户端、插件或工作流里。',
          },
          {
            icon: 'calculator',
            title: '按用量理解成本',
            body: '不同模型价格和消耗不同。长上下文、图片、复杂推理通常会消耗更多额度。',
          },
          {
            icon: 'chat',
            title: '遇到失败先换模型',
            body: '如果某个模型临时不可用，可以切换其他可用模型；平台会尽量做路由和故障切换。',
          },
          {
            icon: 'shield',
            title: '密钥泄露立即处理',
            body: '如果怀疑密钥泄露，请立刻删除或禁用旧密钥，再创建新的密钥继续使用。',
          },
        ],
      },
      safeUse: {
        kicker: '使用提醒',
        title: '稳定使用的几个习惯',
        items: [
          '不要把 API 密钥发到群聊、截图、公开仓库或不可信工具里。',
          '大批量任务先小规模测试，确认模型、成本和输出质量再放大。',
          '团队共用时尽量给不同成员或项目创建不同密钥，方便追踪用量。',
          '如果账号、额度、支付或模型访问异常，先截图错误信息再联系管理员。',
        ],
      },
      faq: {
        kicker: '常见问题',
        title: '你可能会问',
        items: [
          {
            q: '我应该选哪个模型？',
            a: '写代码和工具调用优先试 GPT 或 Claude；长文档可以试 Kimi、Claude、Gemini；成本敏感可以试 DeepSeek 或其他低价模型。',
          },
          {
            q: '为什么同样的问题消耗不一样？',
            a: '消耗和输入长度、输出长度、模型价格、是否使用图片或长上下文有关。越长、越复杂，通常越贵。',
          },
          {
            q: 'API 密钥丢了还能找回吗？',
            a: '通常不能再次查看完整密钥。你可以删除旧密钥，重新创建一个新的。',
          },
          {
            q: '请求失败一定是平台坏了吗？',
            a: '不一定。可能是模型临时不可用、余额不足、密钥填错、客户端配置错或上游限流。先看错误提示和用量记录。',
          },
        ],
      },
    }
  }

  return {
    kicker: `${siteName.value} Guide`,
    title: 'Use one API key as your AI work entry point.',
    description: 'This guide is for people using this website: sign in, get a key, choose models, manage balance or plans, and check usage. No codebase or deployment knowledge required.',
    login: 'Login',
    dashboard: 'Dashboard',
    openDashboard: 'Open Dashboard',
    startNow: 'Start Now',
    readSteps: 'Read Steps',
    switchToLight: 'Switch to Light Mode',
    switchToDark: 'Switch to Dark Mode',
    pageNav: 'Documentation navigation',
    allRightsReserved: 'All rights reserved.',
    backHome: 'Back Home',
    nav: {
      quickStart: 'Quick Start',
      dailyUse: 'Daily Use',
      faq: 'FAQ',
    },
    quickStart: {
      kicker: 'Five steps',
      title: 'From sign-in to your first request',
    },
    steps: [
      {
        icon: 'login',
        title: 'Sign in or create an account',
        body: 'Use the account provided by your administrator, or create one yourself if registration is enabled.',
      },
      {
        icon: 'creditCard',
        title: 'Check balance or plan',
        body: 'Open the dashboard to confirm your balance, active plan, and available models. Add balance or redeem a code before heavy use.',
      },
      {
        icon: 'key',
        title: 'Create an API key',
        body: 'Create your own key on the API Keys page. Copy it once and keep it private.',
      },
      {
        icon: 'brain',
        title: 'Choose a model',
        body: 'Pick GPT, Claude, Gemini, DeepSeek, or another available model based on coding, long documents, reasoning, or multimodal needs.',
      },
      {
        icon: 'chartBar',
        title: 'Track usage',
        body: 'Use the usage pages to review requests, cost, and remaining quota. Disable a key quickly if anything looks abnormal.',
      },
    ],
    daily: {
      kicker: 'Daily use',
      title: 'Treat the platform as one AI gateway',
      items: [
        {
          icon: 'copy',
          title: 'Paste the key into your tool',
          body: 'Use the key in clients, plugins, or workflows that support OpenAI, Claude, or Gemini-compatible configuration.',
        },
        {
          icon: 'calculator',
          title: 'Understand cost by usage',
          body: 'Different models cost different amounts. Long context, images, and complex reasoning usually consume more quota.',
        },
        {
          icon: 'chat',
          title: 'Switch models when needed',
          body: 'If one model is temporarily unavailable, try another available model while the platform handles routing and failover.',
        },
        {
          icon: 'shield',
          title: 'Rotate leaked keys',
          body: 'If you suspect a key was exposed, delete or disable it immediately and create a new one.',
        },
      ],
    },
    safeUse: {
      kicker: 'Good habits',
      title: 'A few ways to keep things stable',
      items: [
        'Do not share API keys in chats, screenshots, public repositories, or untrusted tools.',
        'Test batch jobs at a small scale before increasing volume.',
        'Use separate keys for different people or projects so usage is easy to trace.',
        'When account, quota, payment, or model access fails, capture the error and contact an administrator.',
      ],
    },
    faq: {
      kicker: 'FAQ',
      title: 'Common questions',
      items: [
        {
          q: 'Which model should I use?',
          a: 'For coding and tool workflows, try GPT or Claude. For long documents, try Kimi, Claude, or Gemini. For cost-sensitive work, try DeepSeek or another lower-cost model.',
        },
        {
          q: 'Why does the same task cost different amounts?',
          a: 'Cost depends on input length, output length, model pricing, images, and long-context usage. Longer and more complex work usually costs more.',
        },
        {
          q: 'Can I recover a lost API key?',
          a: 'Usually no. Delete the old key and create a new one.',
        },
        {
          q: 'Does a failed request always mean the platform is broken?',
          a: 'Not always. It may be temporary model availability, insufficient balance, a wrong key, client configuration, or upstream rate limits.',
        },
      ],
    },
  }
})

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.topai-docs-page {
  min-height: 100vh;
  color: #102035;
  background:
    radial-gradient(circle at 50% 0%, rgba(45, 212, 191, 0.18), transparent 34rem),
    linear-gradient(90deg, rgba(10, 18, 29, 0.08) 1px, transparent 1px),
    linear-gradient(0deg, rgba(10, 18, 29, 0.08) 1px, transparent 1px),
    #f7f7f5;
  background-size: auto, 64px 64px, 64px 64px, auto;
}

.topai-docs-page.is-dark {
  color: #eafffc;
  background:
    radial-gradient(circle at 50% 0%, rgba(45, 255, 229, 0.16), transparent 34rem),
    linear-gradient(90deg, rgba(148, 163, 184, 0.1) 1px, transparent 1px),
    linear-gradient(0deg, rgba(148, 163, 184, 0.1) 1px, transparent 1px),
    #07111f;
  background-size: auto, 64px 64px, 64px 64px, auto;
}

.topai-docs-header {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 18px 28px;
  border-bottom: 1px solid rgba(37, 99, 235, 0.12);
  background: rgba(247, 247, 245, 0.76);
  backdrop-filter: blur(18px);
}

.topai-docs-page.is-dark .topai-docs-header {
  border-bottom-color: rgba(92, 255, 236, 0.14);
  background: rgba(7, 17, 31, 0.78);
}

.topai-docs-brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  color: inherit;
  text-decoration: none;
}

.topai-docs-logo-wrap {
  display: inline-flex;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(37, 99, 235, 0.24);
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(240, 253, 250, 0.9), rgba(219, 234, 254, 0.72));
  box-shadow: 0 0 26px rgba(45, 212, 191, 0.14);
}

.topai-docs-page.is-dark .topai-docs-logo-wrap {
  border-color: rgba(92, 255, 236, 0.28);
  background: rgba(255, 255, 255, 0.06);
}

.topai-docs-logo {
  width: 32px;
  height: 32px;
  object-fit: contain;
}

.topai-docs-brand-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  line-height: 1.15;
}

.topai-docs-brand-name {
  font-size: 16px;
  font-weight: 900;
  letter-spacing: 0.06em;
}

.topai-docs-brand-subtitle {
  max-width: 260px;
  overflow: hidden;
  color: rgba(71, 85, 105, 0.9);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topai-docs-page.is-dark .topai-docs-brand-subtitle {
  color: rgba(204, 251, 241, 0.66);
}

.topai-docs-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.topai-docs-icon-button,
.topai-docs-primary-link,
.topai-docs-cta,
.topai-docs-secondary-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px solid rgba(37, 99, 235, 0.24);
  border-radius: 9px;
  color: inherit;
  font-size: 12px;
  font-weight: 800;
  text-decoration: none;
  transition: border-color 0.16s ease, background 0.16s ease, transform 0.16s ease;
}

.topai-docs-icon-button {
  width: 36px;
  height: 36px;
  background: rgba(255, 255, 255, 0.54);
}

.topai-docs-primary-link {
  height: 36px;
  padding: 0 13px;
  background: rgba(45, 212, 191, 0.14);
  text-transform: uppercase;
}

.topai-docs-page.is-dark .topai-docs-icon-button,
.topai-docs-page.is-dark .topai-docs-primary-link,
.topai-docs-page.is-dark .topai-docs-cta,
.topai-docs-page.is-dark .topai-docs-secondary-link {
  border-color: rgba(92, 255, 236, 0.28);
  background: rgba(255, 255, 255, 0.07);
}

.topai-docs-icon-button:hover,
.topai-docs-primary-link:hover,
.topai-docs-cta:hover,
.topai-docs-secondary-link:hover {
  border-color: rgba(37, 99, 235, 0.46);
  transform: translateY(-1px);
}

.topai-docs-user-mark {
  display: inline-flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.16);
}

.topai-docs-main {
  width: min(1120px, calc(100% - 40px));
  margin: 0 auto;
  padding: 72px 0 90px;
}

.topai-docs-hero {
  max-width: 780px;
}

.topai-docs-kicker,
.topai-docs-section-heading p {
  margin: 0 0 14px;
  color: #0f766e;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 900;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.topai-docs-page.is-dark .topai-docs-kicker,
.topai-docs-page.is-dark .topai-docs-section-heading p {
  color: #5eead4;
}

.topai-docs-hero h1 {
  margin: 0;
  max-width: 760px;
  font-size: clamp(42px, 7vw, 86px);
  font-weight: 950;
  letter-spacing: 0;
  line-height: 0.96;
}

.topai-docs-hero > p {
  max-width: 680px;
  margin: 24px 0 0;
  color: rgba(51, 65, 85, 0.86);
  font-size: 18px;
  line-height: 1.8;
}

.topai-docs-page.is-dark .topai-docs-hero > p {
  color: rgba(226, 252, 248, 0.76);
}

.topai-docs-hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 32px;
}

.topai-docs-cta,
.topai-docs-secondary-link {
  height: 42px;
  padding: 0 18px;
}

.topai-docs-cta {
  border-color: rgba(15, 23, 42, 0.12);
  background: #06111f;
  color: #ecfeff;
}

.topai-docs-secondary-link {
  background: rgba(255, 255, 255, 0.5);
}

.topai-docs-nav {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 58px 0 72px;
}

.topai-docs-nav a {
  border: 1px solid rgba(20, 184, 166, 0.24);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.52);
  color: inherit;
  padding: 9px 14px;
  font-size: 13px;
  font-weight: 800;
  text-decoration: none;
}

.topai-docs-page.is-dark .topai-docs-nav a {
  background: rgba(255, 255, 255, 0.06);
}

.topai-docs-section {
  margin-top: 78px;
}

.topai-docs-section-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.topai-docs-section-heading h2,
.topai-docs-note-section h2 {
  margin: 0;
  font-size: clamp(28px, 4vw, 44px);
  font-weight: 950;
  letter-spacing: 0;
}

.topai-docs-steps,
.topai-docs-use-grid,
.topai-docs-faq {
  display: grid;
  gap: 16px;
}

.topai-docs-steps {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.topai-docs-use-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.topai-docs-step,
.topai-docs-use-card,
.topai-docs-faq article {
  position: relative;
  min-height: 100%;
  border: 1px solid rgba(30, 64, 175, 0.14);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.62);
  padding: 22px;
  box-shadow: 0 22px 80px rgba(15, 23, 42, 0.06);
}

.topai-docs-page.is-dark .topai-docs-step,
.topai-docs-page.is-dark .topai-docs-use-card,
.topai-docs-page.is-dark .topai-docs-faq article {
  border-color: rgba(92, 255, 236, 0.14);
  background: rgba(7, 18, 32, 0.72);
  box-shadow: 0 22px 80px rgba(0, 0, 0, 0.22);
}

.topai-docs-step-index {
  position: absolute;
  top: 14px;
  right: 16px;
  color: rgba(37, 99, 235, 0.32);
  font-size: 28px;
  font-weight: 950;
}

.topai-docs-step-icon,
.topai-docs-use-icon {
  display: inline-flex;
  width: 42px;
  height: 42px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.16), rgba(59, 130, 246, 0.14));
  color: #0f766e;
}

.topai-docs-page.is-dark .topai-docs-step-icon,
.topai-docs-page.is-dark .topai-docs-use-icon {
  color: #67e8f9;
}

.topai-docs-step h3,
.topai-docs-use-card h3,
.topai-docs-faq h3 {
  margin: 18px 0 10px;
  font-size: 17px;
  font-weight: 900;
}

.topai-docs-step p,
.topai-docs-use-card p,
.topai-docs-faq p {
  margin: 0;
  color: rgba(51, 65, 85, 0.82);
  font-size: 14px;
  line-height: 1.75;
}

.topai-docs-page.is-dark .topai-docs-step p,
.topai-docs-page.is-dark .topai-docs-use-card p,
.topai-docs-page.is-dark .topai-docs-faq p {
  color: rgba(226, 252, 248, 0.72);
}

.topai-docs-note-section {
  display: grid;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr);
  gap: 34px;
  align-items: start;
  border-top: 1px solid rgba(20, 184, 166, 0.2);
  border-bottom: 1px solid rgba(20, 184, 166, 0.2);
  padding: 42px 0;
}

.topai-docs-note-section ul {
  display: grid;
  gap: 14px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.topai-docs-note-section li {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  color: rgba(51, 65, 85, 0.88);
  font-size: 15px;
  line-height: 1.7;
}

.topai-docs-page.is-dark .topai-docs-note-section li {
  color: rgba(226, 252, 248, 0.78);
}

.topai-docs-note-section li svg {
  flex: 0 0 auto;
  margin-top: 4px;
  color: #0f766e;
}

.topai-docs-faq {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.topai-docs-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  width: min(1120px, calc(100% - 40px));
  margin: 0 auto;
  padding: 28px 0 34px;
  color: rgba(71, 85, 105, 0.78);
  font-size: 13px;
}

.topai-docs-footer a {
  color: inherit;
  font-weight: 800;
  text-decoration: none;
}

.topai-docs-page.is-dark .topai-docs-footer {
  color: rgba(204, 251, 241, 0.62);
}

@media (max-width: 980px) {
  .topai-docs-steps,
  .topai-docs-use-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .topai-docs-header {
    align-items: flex-start;
    flex-direction: column;
    padding: 14px;
  }

  .topai-docs-actions {
    width: 100%;
    justify-content: space-between;
  }

  .topai-docs-brand-subtitle {
    max-width: calc(100vw - 110px);
  }

  .topai-docs-main {
    width: min(100% - 28px, 1120px);
    padding: 44px 0 68px;
  }

  .topai-docs-hero h1 {
    font-size: 42px;
  }

  .topai-docs-hero > p {
    font-size: 16px;
  }

  .topai-docs-section-heading,
  .topai-docs-note-section {
    display: block;
  }

  .topai-docs-steps,
  .topai-docs-use-grid,
  .topai-docs-faq {
    grid-template-columns: 1fr;
  }

  .topai-docs-note-section ul {
    margin-top: 22px;
  }

  .topai-docs-footer {
    align-items: flex-start;
    flex-direction: column;
    width: min(100% - 28px, 1120px);
  }
}
</style>
