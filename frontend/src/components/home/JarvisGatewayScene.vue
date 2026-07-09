<template>
  <main class="jarvis-page" :class="{ 'is-dark': isDarkTheme }">
    <canvas ref="canvasRef" class="jarvis-canvas" aria-hidden="true"></canvas>

    <div class="jarvis-vignette" aria-hidden="true"></div>
    <div class="jarvis-grid" aria-hidden="true"></div>

    <section class="jarvis-stage" aria-label="Sub2API neural gateway concept">
      <div v-if="showTopbar" class="jarvis-topbar">
        <slot name="brand">
          <div class="jarvis-brand">
            <span class="jarvis-brand-mark">S2</span>
            <span>
              <strong>Sub2API</strong>
              <em>Neural Gateway</em>
            </span>
          </div>
        </slot>
        <slot name="topbar-end">
          <div class="jarvis-status">
            <span class="jarvis-dot"></span>
            LIVE ROUTING CORE
          </div>
        </slot>
      </div>

      <div class="jarvis-core">
        <div class="jarvis-orbit jarvis-orbit-one">
          <span></span>
          <span></span>
        </div>
        <div class="jarvis-orbit jarvis-orbit-two">
          <span></span>
          <span></span>
          <span></span>
        </div>
        <div class="jarvis-energy-wave jarvis-energy-wave-a"></div>
        <div class="jarvis-energy-wave jarvis-energy-wave-b"></div>
        <div class="jarvis-ring jarvis-ring-a"></div>
        <div class="jarvis-ring jarvis-ring-b"></div>
        <div class="jarvis-ring jarvis-ring-c"></div>
        <div class="jarvis-logo-system" aria-label="Top AI model constellation">
          <div
            v-for="model in modelLogos"
            :key="model.name"
            class="jarvis-logo-orbit"
            :class="[
              `jarvis-logo-orbit-${model.ring}`,
              { 'is-hovered': hoveredLogoName === model.name }
            ]"
            :data-logo="model.name"
            :style="{
              '--angle': `${model.angle}deg`,
              '--radius': `${model.radius}px`,
              '--duration': `${model.duration}s`,
              '--delay': `${model.delay}s`,
              '--tilt': `${model.tilt}deg`
            }"
          >
            <span
              class="jarvis-model-logo"
              :class="{ 'jarvis-model-logo-text': model.kind === 'text' }"
              :title="`${model.name} · ${model.model}`"
              :style="{
                '--brand': model.color,
                '--brand-soft': model.softColor
              }"
            >
              <svg
                v-if="model.paths"
                viewBox="0 0 24 24"
                aria-hidden="true"
              >
                <path
                  v-for="path in model.paths"
                  :key="path"
                  :d="path"
                  fill="currentColor"
                  fill-rule="evenodd"
                />
              </svg>
              <span v-else>{{ model.mark }}</span>
            </span>
          </div>
        </div>
        <div class="jarvis-brain">
          <span class="jarvis-brain-scan"></span>
          <span class="jarvis-brain-label">API</span>
        </div>
      </div>

      <Transition name="jarvis-chat">
        <aside
          v-if="hoveredModel"
          class="jarvis-model-chat"
          :style="{
            '--chat-x': `${chatPosition.x}px`,
            '--chat-y': `${chatPosition.y}px`,
            '--brand': hoveredModel.color,
            '--brand-soft': hoveredModel.softColor
          }"
          aria-live="polite"
        >
          <div class="jarvis-chat-head">
            <div>
              <strong>{{ hoveredModel.name }}</strong>
              <em>{{ hoveredModel.model }}</em>
            </div>
          </div>
          <p>
            {{ typedModelText }}<span class="jarvis-type-cursor"></span>
          </p>
          <div class="jarvis-chat-foot">
            <span>{{ hoveredModel.specialty }}</span>
            <span>FOCUS LINKED</span>
          </div>
        </aside>
      </Transition>

      <div v-if="showCopy" class="jarvis-copy">
        <p class="jarvis-kicker">{{ copyKicker }}</p>
        <h1>{{ copyTitle }}</h1>
        <p>
          {{ copyDescription }}
        </p>
      </div>

      <aside v-if="showPanels" class="jarvis-panel jarvis-panel-left">
        <div class="jarvis-panel-heading">
          <span>ROUTE MATRIX</span>
          <b>{{ matrixHealth.toFixed(2) }}%</b>
        </div>
        <ul>
          <li v-for="metric in routeMetrics" :key="metric.name">
            <span>{{ metric.name }}</span>
            <strong>{{ Math.round(metric.latency) }}ms</strong>
          </li>
        </ul>
      </aside>

      <aside v-if="showPanels" class="jarvis-panel jarvis-panel-right">
        <div class="jarvis-panel-heading">
          <span>LOAD SIGNAL</span>
          <b>LIVE</b>
        </div>
        <div class="jarvis-bars">
          <span
            v-for="(bar, index) in loadBars"
            :key="index"
            :style="{ '--h': `${Math.round(bar)}%` }"
          ></span>
        </div>
        <code>POST /v1/responses -> optimal_lane</code>
      </aside>

      <div v-if="showBottomHud" class="jarvis-bottom-hud">
        <span>request stream: {{ requestStream.toFixed(1) }}k/min</span>
        <span>fallback armed</span>
        <span>quota sync {{ Math.round(quotaSync) }}</span>
        <span>cache hit {{ cacheHit.toFixed(1) }}%</span>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{
  showTopbar?: boolean
  showCopy?: boolean
  showPanels?: boolean
  showBottomHud?: boolean
  copyKicker?: string
  copyTitle?: string
  copyDescription?: string
}>(), {
  showTopbar: true,
  showCopy: true,
  showPanels: true,
  showBottomHud: true,
  copyKicker: 'AI ROUTING INTELLIGENCE',
  copyTitle: 'Model traffic, thinking in motion.',
  copyDescription: 'One gateway coordinates upstream accounts, latency, quotas, fallback paths, and billing signals as a living command system.'
})

type NodePoint = {
  x: number
  y: number
  z: number
  baseX: number
  baseY: number
  baseZ: number
  offsetX: number
  offsetY: number
  dragX: number | null
  dragY: number | null
  phase: number
  speed: number
  radius: number
  hue: number
}

type Pulse = {
  from: number
  to: number
  progress: number
  speed: number
}

type ModelLogo = {
  rank: number
  name: string
  model: string
  mark: string
  specialty: string
  intro: string
  color: string
  softColor: string
  ring: 'inner' | 'middle' | 'outer'
  angle: number
  radius: number
  duration: number
  delay: number
  tilt: number
  kind?: 'text'
  paths?: string[]
}

type RouteMetric = {
  name: string
  latency: number
  min: number
  max: number
  step: number
}

const { t } = useI18n()

const providerPaths = {
  openai: [
    'M21.55 10.004a5.416 5.416 0 00-.478-4.501c-1.217-2.09-3.662-3.166-6.05-2.66A5.59 5.59 0 0010.831 1C8.39.995 6.224 2.546 5.473 4.838A5.553 5.553 0 001.76 7.496a5.487 5.487 0 00.691 6.5 5.416 5.416 0 00.477 4.502c1.217 2.09 3.662 3.165 6.05 2.66A5.586 5.586 0 0013.168 23c2.443.006 4.61-1.546 5.361-3.84a5.553 5.553 0 003.715-2.66 5.488 5.488 0 00-.693-6.497v.001zm-8.381 11.558a4.199 4.199 0 01-2.675-.954c.034-.018.093-.05.132-.074l4.44-2.53a.71.71 0 00.364-.623v-6.176l1.877 1.069c.02.01.033.029.036.05v5.115c-.003 2.274-1.87 4.118-4.174 4.123zM4.192 17.78a4.059 4.059 0 01-.498-2.763c.032.02.09.055.131.078l4.44 2.53c.225.13.504.13.73 0l5.42-3.088v2.138a.068.068 0 01-.027.057L9.9 19.288c-1.999 1.136-4.552.46-5.707-1.51h-.001zM3.023 8.216A4.15 4.15 0 015.198 6.41l-.002.151v5.06a.711.711 0 00.364.624l5.42 3.087-1.876 1.07a.067.067 0 01-.063.005l-4.489-2.559c-1.995-1.14-2.679-3.658-1.53-5.63h.001zm15.417 3.54l-5.42-3.088L14.896 7.6a.067.067 0 01.063-.006l4.489 2.557c1.998 1.14 2.683 3.662 1.529 5.633a4.163 4.163 0 01-2.174 1.807V12.38a.71.71 0 00-.363-.623zm1.867-2.773a6.04 6.04 0 00-.132-.078l-4.44-2.53a.731.731 0 00-.729 0l-5.42 3.088V7.325a.068.068 0 01.027-.057L14.1 4.713c2-1.137 4.555-.46 5.707 1.513.487.833.664 1.809.499 2.757h.001zm-11.741 3.81l-1.877-1.068a.065.065 0 01-.036-.051V6.559c.001-2.277 1.873-4.122 4.181-4.12.976 0 1.92.338 2.671.954-.034.018-.092.05-.131.073l-4.44 2.53a.71.71 0 00-.365.623l-.003 6.173v.002zm1.02-2.168L12 9.25l2.414 1.375v2.75L12 14.75l-2.415-1.375v-2.75z'
  ],
  anthropic: [
    'M4.709 15.955l4.72-2.647.08-.23-.08-.128H9.2l-.79-.048-2.698-.073-2.339-.097-2.266-.122-.571-.121L0 11.784l.055-.352.48-.321.686.06 1.52.103 2.278.158 1.652.097 2.449.255h.389l.055-.157-.134-.098-.103-.097-2.358-1.596-2.552-1.688-1.336-.972-.724-.491-.364-.462-.158-1.008.656-.722.881.06.225.061.893.686 1.908 1.476 2.491 1.833.365.304.145-.103.019-.073-.164-.274-1.355-2.446-1.446-2.49-.644-1.032-.17-.619a2.97 2.97 0 01-.104-.729L6.283.134 6.696 0l.996.134.42.364.62 1.414 1.002 2.229 1.555 3.03.456.898.243.832.091.255h.158V9.01l.128-1.706.237-2.095.23-2.695.08-.76.376-.91.747-.492.584.28.48.685-.067.444-.286 1.851-.559 2.903-.364 1.942h.212l.243-.242.985-1.306 1.652-2.064.73-.82.85-.904.547-.431h1.033l.76 1.129-.34 1.166-1.064 1.347-.881 1.142-1.264 1.7-.79 1.36.073.11.188-.02 2.856-.606 1.543-.28 1.841-.315.833.388.091.395-.328.807-1.969.486-2.309.462-3.439.813-.042.03.049.061 1.549.146.662.036h1.622l3.02.225.79.522.474.638-.079.485-1.215.62-1.64-.389-3.829-.91-1.312-.329h-.182v.11l1.093 1.068 2.006 1.81 2.509 2.33.127.578-.322.455-.34-.049-2.205-1.657-.851-.747-1.926-1.62h-.128v.17l.444.649 2.345 3.521.122 1.08-.17.353-.608.213-.668-.122-1.374-1.925-1.415-2.167-1.143-1.943-.14.08-.674 7.254-.316.37-.729.28-.607-.461-.322-.747.322-1.476.389-1.924.315-1.53.286-1.9.17-.632-.012-.042-.14.018-1.434 1.967-2.18 2.945-1.726 1.845-.414.164-.717-.37.067-.662.401-.589 2.388-3.036 1.44-1.882.93-1.086-.006-.158h-.055L4.132 18.56l-1.13.146-.487-.456.061-.746.231-.243 1.908-1.312-.006.006z'
  ],
  gemini: [
    'M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z'
  ]
}

const baseModelLogos: ModelLogo[] = [
  { rank: 1, name: 'Claude', model: 'Opus / Sonnet', mark: 'C', specialty: '长文 · 推理 · 写作', intro: '我是 Claude，写报告像咨询顾问，讲道理像班主任，贵是贵点，但我真的稳。', color: '#d97757', softColor: 'rgba(217, 119, 87, 0.3)', ring: 'middle', angle: 34, radius: 156, duration: 26, delay: -5.4, tilt: -6, paths: providerPaths.anthropic },
  { rank: 2, name: 'OpenAI', model: 'GPT / o-series', mark: 'AI', specialty: '工具调用 · 代码 · Agent', intro: '我是 OpenAI，全场最会接工具的打工人，写代码、调 API、跑 Agent，我一条龙。', color: '#10a37f', softColor: 'rgba(16, 163, 127, 0.3)', ring: 'inner', angle: 12, radius: 108, duration: 19, delay: -1.5, tilt: 4, paths: providerPaths.openai },
  { rank: 3, name: 'Gemini', model: 'Gemini Pro', mark: 'G', specialty: '多模态 · 长上下文 · Google 生态', intro: '我是 Gemini，文字图片视频都能看，背后还有 Google，主打一个家里资源多。', color: '#8ab4f8', softColor: 'rgba(138, 180, 248, 0.34)', ring: 'outer', angle: 53, radius: 205, duration: 34, delay: -10.5, tilt: 7, paths: providerPaths.gemini },
  { rank: 4, name: 'Grok', model: 'Grok 4 / 3', mark: 'xAI', specialty: '实时感 · 个性 · 快速推理', intro: '我是 Grok，别人一本正经，我先开个玩笑；能聊能冲，就是嘴比较快。', color: '#f4f4f5', softColor: 'rgba(244, 244, 245, 0.28)', ring: 'inner', angle: 88, radius: 122, duration: 21, delay: -9.2, tilt: -3, kind: 'text' },
  { rank: 5, name: 'Qwen', model: 'Qwen3', mark: 'Q', specialty: '中文 · 代码 · 开源', intro: '我是 Qwen，中文我熟，代码我会，开源我给，老板问成本我也不慌。', color: '#615ced', softColor: 'rgba(97, 92, 237, 0.34)', ring: 'outer', angle: 135, radius: 214, duration: 35, delay: -18.3, tilt: -8, kind: 'text' },
  { rank: 6, name: 'DeepSeek', model: 'DeepSeek R1/V3', mark: 'DS', specialty: '推理 · 代码 · 低成本', intro: '我是 DeepSeek，我会推理，还便宜，还能打。你预算紧？巧了，我也擅长省钱。', color: '#4d6bfe', softColor: 'rgba(77, 107, 254, 0.34)', ring: 'middle', angle: 112, radius: 168, duration: 27, delay: -12.1, tilt: 9, kind: 'text' },
  { rank: 7, name: 'Kimi', model: 'Kimi K2', mark: 'K', specialty: '长上下文 · 文档 · 中文', intro: '我是 Kimi，你把文档堆过来，我慢慢吃。长文档？别客气，我胃口大。', color: '#00d4ff', softColor: 'rgba(0, 212, 255, 0.34)', ring: 'outer', angle: 213, radius: 198, duration: 36, delay: -6.2, tilt: 5, kind: 'text' },
  { rank: 8, name: 'Meta', model: 'Llama 4', mark: '∞', specialty: '开源 · 本地部署 · 微调', intro: '我是 Llama，不一定最会装，但最适合被你改装。想私有化？我搬进你服务器。', color: '#0866ff', softColor: 'rgba(8, 102, 255, 0.34)', ring: 'inner', angle: 166, radius: 112, duration: 22, delay: -4.8, tilt: 6, kind: 'text' },
  { rank: 9, name: 'Zhipu', model: 'GLM', mark: 'GLM', specialty: '中文推理 · 企业 · 国产生态', intro: '我是 GLM，中文业务我门儿清，写方案、做问答、进企业，我比较接地气。', color: '#006fff', softColor: 'rgba(0, 111, 255, 0.3)', ring: 'inner', angle: 244, radius: 126, duration: 20, delay: -16.4, tilt: -4, kind: 'text' },
  { rank: 10, name: 'Baidu', model: 'ERNIE', mark: 'du', specialty: '中文知识 · 搜索增强 · 落地', intro: '我是 ERNIE，背靠百度搜索，别人靠记忆，我还能顺手翻资料。', color: '#2932e1', softColor: 'rgba(41, 50, 225, 0.34)', ring: 'middle', angle: 268, radius: 174, duration: 29, delay: -2.1, tilt: 9, kind: 'text' },
  { rank: 11, name: 'MiniMax', model: 'M1', mark: 'MM', specialty: '中文对话 · 长上下文 · 应用体验', intro: '我是 MiniMax，聊天不端着，长文也能扛，主打一个产品感比较顺。', color: '#f14d5d', softColor: 'rgba(241, 77, 93, 0.32)', ring: 'outer', angle: 252, radius: 218, duration: 40, delay: -15.5, tilt: 8, kind: 'text' },
  { rank: 12, name: 'Mistral', model: 'Large / Medium', mark: 'M', specialty: '轻量高效 · 商用 · 欧洲生态', intro: '我是 Mistral，法国来的效率派，不一定嗓门最大，但跑起来很利索。', color: '#ff7000', softColor: 'rgba(255, 112, 0, 0.32)', ring: 'middle', angle: 193, radius: 160, duration: 28, delay: -15.6, tilt: -9, kind: 'text' },
  { rank: 13, name: 'AWS', model: 'Nova', mark: 'AWS', specialty: '云集成 · 企业部署 · 稳定', intro: '我是 Nova，AWS 家亲儿子，上云、扩容、接企业系统，我熟得像回家。', color: '#ff9900', softColor: 'rgba(255, 153, 0, 0.32)', ring: 'inner', angle: 318, radius: 116, duration: 23, delay: -12.9, tilt: 4, kind: 'text' },
  { rank: 14, name: 'Hunyuan', model: 'Tencent Hunyuan', mark: 'H', specialty: '中文 · 多媒体 · 腾讯生态', intro: '我是 Hunyuan，中文内容和社交场景我懂，毕竟腾讯系流量我见得多。', color: '#217aff', softColor: 'rgba(33, 122, 255, 0.34)', ring: 'outer', angle: 291, radius: 207, duration: 37, delay: -22.5, tilt: -7, kind: 'text' },
  { rank: 15, name: 'Nvidia', model: 'Nemotron', mark: 'NV', specialty: '推理优化 · GPU · 私有化', intro: '我是 Nemotron，别人谈模型，我先看显卡。GPU 吃满，性能拉满。', color: '#76b900', softColor: 'rgba(118, 185, 0, 0.32)', ring: 'middle', angle: 229, radius: 187, duration: 32, delay: -24.4, tilt: -6, kind: 'text' },
  { rank: 16, name: 'Doubao', model: 'Seed / Doubao', mark: '豆', specialty: '中文内容 · 对话 · 产品化', intro: '我是 Doubao，做内容我熟，用户爱不爱看，我比你老板还敏感。', color: '#4f7cff', softColor: 'rgba(79, 124, 255, 0.34)', ring: 'middle', angle: 306, radius: 163, duration: 33, delay: -6.8, tilt: -5, kind: 'text' },
  { rank: 17, name: 'Microsoft', model: 'Phi', mark: 'Φ', specialty: '小模型 · 高效率 · 端侧', intro: '我是 Phi，别看我小，我省电省钱省机器。小身板，也能干正事。', color: '#7fba00', softColor: 'rgba(127, 186, 0, 0.32)', ring: 'outer', angle: 151, radius: 229, duration: 39, delay: -3.4, tilt: 6, kind: 'text' },
  { rank: 18, name: 'Perplexity', model: 'Sonar', mark: 'P', specialty: '联网搜索 · 引用 · 研究', intro: '我是 Sonar，我不装全知全能，我会查，还会告诉你我从哪查的。', color: '#20b8cd', softColor: 'rgba(32, 184, 205, 0.33)', ring: 'middle', angle: 64, radius: 181, duration: 31, delay: -9.8, tilt: -8, kind: 'text' },
  { rank: 19, name: 'AI21', model: 'Jamba', mark: '21', specialty: '长上下文 · 文档 · 企业文本', intro: '我是 Jamba，文档工作流我能啃，合同、报告、知识库，来多少我排队处理。', color: '#ff4f00', softColor: 'rgba(255, 79, 0, 0.3)', ring: 'outer', angle: 9, radius: 220, duration: 38, delay: -27.1, tilt: 7, kind: 'text' },
  { rank: 20, name: 'Cohere', model: 'Command', mark: 'co', specialty: 'RAG · 多语言 · 企业检索', intro: '我是 Cohere，企业 RAG 老熟人，别问我会不会聊天，问我资料找得准不准。', color: '#39594d', softColor: 'rgba(57, 89, 77, 0.32)', ring: 'middle', angle: 339, radius: 153, duration: 30, delay: -19.2, tilt: -5, kind: 'text' }
]

const modelLogos = computed<ModelLogo[]>(() => {
  return baseModelLogos.map((model) => {
    const keyPrefix = `home.jarvis.models.${model.name}`
    const specialtyKey = `${keyPrefix}.specialty`
    const introKey = `${keyPrefix}.intro`
    const specialty = t(specialtyKey)
    const intro = t(introKey)

    return {
      ...model,
      specialty: specialty === specialtyKey ? model.specialty : specialty,
      intro: intro === introKey ? model.intro : intro
    }
  })
})

const canvasRef = ref<HTMLCanvasElement | null>(null)
const hoveredLogoName = ref<string | null>(null)
const typedModelText = ref('')
const chatPosition = ref({ x: 0, y: 0 })
const hoveredModel = computed(() => modelLogos.value.find((model) => model.name === hoveredLogoName.value) ?? null)
const isDarkTheme = ref(false)
const matrixHealth = ref(99.98)
const requestStream = ref(18.4)
const quotaSync = ref(2048)
const cacheHit = ref(98.0)
const routeMetrics = ref<RouteMetric[]>([
  { name: 'OpenAI', latency: 42, min: 39, max: 48, step: 2.2 },
  { name: 'Claude', latency: 61, min: 56, max: 66, step: 2.4 },
  { name: 'Gemini', latency: 55, min: 51, max: 62, step: 2.1 },
  { name: 'Antigravity', latency: 33, min: 29, max: 38, step: 1.9 }
])
const loadBars = ref([66, 38, 82, 52, 74, 45, 90])
let animationFrame = 0
let width = 0
let height = 0
let centerX = 0
let centerY = 0
let nodes: NodePoint[] = []
let pulses: Pulse[] = []
let reduceMotion = false
let resizeHandler: (() => void) | null = null
let pointerMoveHandler: ((event: PointerEvent) => void) | null = null
let pointerDownHandler: ((event: PointerEvent) => void) | null = null
let pointerUpHandler: (() => void) | null = null
let pointerLeaveHandler: (() => void) | null = null
let visibilityHandler: (() => void) | null = null
let pointerX = 0
let pointerY = 0
let targetPointerX = 0
let targetPointerY = 0
let pointerActive = false
let draggedNodeIndex: number | null = null
let lastProjectedPoints: Array<ReturnType<typeof project>> = []
let typingTimer: number | null = null
let lastRenderTime = 0
let hoverFrame = 0
let pendingHoverPoint: { x: number, y: number } | null = null
let performanceMode: 'balanced' | 'low' = 'balanced'
let targetFrameInterval = 1000 / 36
let themeObserver: MutationObserver | null = null
let telemetryTimer: number | null = null

type NavigatorWithMemory = Navigator & {
  deviceMemory?: number
}

function configurePerformanceProfile() {
  const nav = window.navigator as NavigatorWithMemory
  const cores = nav.hardwareConcurrency ?? 8
  const memory = nav.deviceMemory ?? 8
  const largeViewport = window.innerWidth * window.innerHeight > 2_100_000
  const lowPower = reduceMotion || cores <= 4 || memory <= 4 || largeViewport

  performanceMode = lowPower ? 'low' : 'balanced'
  targetFrameInterval = 1000 / (lowPower ? 24 : 36)
}

function syncThemeState() {
  isDarkTheme.value = document.documentElement.classList.contains('dark')
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function drift(value: number, min: number, max: number, maxStep: number, decimals = 1, minStep = 0) {
  let direction = Math.random() < 0.5 ? -1 : 1
  if (value <= min + minStep) direction = 1
  if (value >= max - minStep) direction = -1
  const magnitude = minStep + Math.random() * Math.max(maxStep - minStep, 0)
  const next = clamp(value + direction * magnitude, min, max)
  const scale = 10 ** decimals
  return Math.round(next * scale) / scale
}

function updateTelemetry() {
  matrixHealth.value = drift(matrixHealth.value, 99.96, 99.99, 0.012, 2, 0.004)
  requestStream.value = drift(requestStream.value, 17.6, 19.8, 0.32, 1, 0.12)
  quotaSync.value = drift(quotaSync.value, 2036, 2064, 3.8, 0, 1)
  cacheHit.value = drift(cacheHit.value, 97.4, 99.2, 0.22, 1, 0.08)
  routeMetrics.value = routeMetrics.value.map((metric) => ({
    ...metric,
    latency: drift(metric.latency, metric.min, metric.max, metric.step, 0, 1)
  }))
  loadBars.value = loadBars.value.map((height) => drift(height, 34, 92, 7, 0, 2))
}

function getNodeCount() {
  return performanceMode === 'low' ? 44 : 64
}

function getPulseCount() {
  return performanceMode === 'low' ? 12 : 20
}

function getCanvasRatioCap() {
  if (reduceMotion) return 1
  return performanceMode === 'low' ? 1.1 : 1.35
}

function getConnectionStride() {
  return performanceMode === 'low' ? 3 : 2
}

function getConnectionDistance() {
  return performanceMode === 'low' ? 104 : 118
}

function stopTypewriter() {
  if (typingTimer !== null) {
    window.clearInterval(typingTimer)
    typingTimer = null
  }
}

function startTypewriter(text: string) {
  stopTypewriter()
  typedModelText.value = ''

  if (!text || reduceMotion) {
    typedModelText.value = text
    return
  }

  let index = 0
  typingTimer = window.setInterval(() => {
    index += 1
    typedModelText.value = text.slice(0, index)
    if (index >= text.length) {
      stopTypewriter()
    }
  }, 18)
}

watch(
  () => hoveredModel.value?.intro ?? '',
  (intro) => startTypewriter(intro),
)

function createNodes() {
  nodes = []
  pulses = []
  const count = getNodeCount()
  const goldenAngle = Math.PI * (3 - Math.sqrt(5))

  for (let i = 0; i < count; i += 1) {
    const y = 1 - (i / (count - 1)) * 2
    const radius = Math.sqrt(1 - y * y)
    const theta = goldenAngle * i
    nodes.push({
      x: Math.cos(theta) * radius,
      y,
      z: Math.sin(theta) * radius,
      baseX: Math.cos(theta) * radius,
      baseY: y,
      baseZ: Math.sin(theta) * radius,
      offsetX: 0,
      offsetY: 0,
      dragX: null,
      dragY: null,
      phase: Math.random() * Math.PI * 2,
      speed: 0.35 + Math.random() * 0.5,
      radius: 1.5 + Math.random() * 2.7,
      hue: 174 + Math.random() * 54
    })
  }

  for (let i = 0; i < getPulseCount(); i += 1) {
    pulses.push({
      from: Math.floor(Math.random() * count),
      to: Math.floor(Math.random() * count),
      progress: Math.random(),
      speed: 0.009 + Math.random() * 0.018
    })
  }
}

function resizeCanvas(canvas: HTMLCanvasElement, ctx: CanvasRenderingContext2D) {
  const ratio = Math.min(window.devicePixelRatio || 1, getCanvasRatioCap())
  width = window.innerWidth
  height = window.innerHeight
  centerX = width / 2
  centerY = height / 2
  canvas.width = Math.floor(width * ratio)
  canvas.height = Math.floor(height * ratio)
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0)
  targetPointerX = centerX
  targetPointerY = centerY
  pointerX = centerX
  pointerY = centerY
}

function project(node: NodePoint, time: number) {
  const pointerTiltX = ((pointerX - centerX) / Math.max(width, 1)) * 0.72
  const pointerTiltY = ((pointerY - centerY) / Math.max(height, 1)) * 0.42
  const spin = time * 0.00034 + pointerTiltX
  const wobble = Math.sin(time * 0.0009 + node.phase) * 0.11
  const sourceX = node.dragX ?? node.x + node.offsetX
  const sourceY = node.dragY ?? node.y + node.offsetY + pointerTiltY
  const sourceZ = node.z
  const x1 = sourceX * Math.cos(spin) - sourceZ * Math.sin(spin)
  const z1 = sourceX * Math.sin(spin) + sourceZ * Math.cos(spin)
  const y1 = sourceY + wobble
  const scaleBase = Math.min(width, height) * 0.25
  const perspective = 1.25 + z1 * 0.42
  return {
    x: centerX + x1 * scaleBase * perspective,
    y: centerY + y1 * scaleBase * 0.72 * perspective,
    z: z1,
    alpha: 0.34 + perspective * 0.36,
    radius: node.radius * perspective
  }
}

function drawBackground(ctx: CanvasRenderingContext2D, time: number) {
  const darkMode = document.documentElement.classList.contains('dark')
  ctx.fillStyle = darkMode ? '#020617' : '#f7f7f5'
  ctx.fillRect(0, 0, width, height)

  const field = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, Math.max(width, height) * 0.72)
  if (darkMode) {
    field.addColorStop(0, 'rgba(2, 6, 17, 0.99)')
    field.addColorStop(0.56, 'rgba(2, 6, 17, 0.96)')
    field.addColorStop(0.82, 'rgba(8, 24, 44, 0.86)')
    field.addColorStop(1, 'rgba(2, 6, 23, 0.98)')
  } else {
    field.addColorStop(0, 'rgba(2, 6, 17, 0.98)')
    field.addColorStop(0.56, 'rgba(2, 6, 17, 0.9)')
    field.addColorStop(0.78, 'rgba(2, 6, 17, 0.28)')
    field.addColorStop(1, 'rgba(247, 247, 245, 0)')
  }
  ctx.fillStyle = field
  ctx.fillRect(0, 0, width, height)

  const scanY = (time * 0.045) % height
  const gradient = ctx.createLinearGradient(0, scanY - 80, 0, scanY + 80)
  gradient.addColorStop(0, 'rgba(0, 255, 220, 0)')
  gradient.addColorStop(0.5, 'rgba(0, 255, 220, 0.08)')
  gradient.addColorStop(1, 'rgba(0, 255, 220, 0)')
  ctx.fillStyle = gradient
  ctx.fillRect(0, scanY - 80, width, 160)
}

function drawConnections(ctx: CanvasRenderingContext2D, points: ReturnType<typeof project>[]) {
  ctx.lineWidth = 1
  const connectionStride = getConnectionStride()
  const connectionDistance = getConnectionDistance()

  for (let i = 0; i < points.length; i += 1) {
    const point = points[i]
    for (let j = i + 1; j < points.length; j += connectionStride) {
      const other = points[j]
      const distance = Math.hypot(point.x - other.x, point.y - other.y)
      if (distance < connectionDistance) {
        const alpha = Math.max(0, (1 - distance / connectionDistance) * 0.28) * Math.min(point.alpha, other.alpha)
        ctx.strokeStyle = `rgba(55, 240, 230, ${alpha})`
        ctx.beginPath()
        ctx.moveTo(point.x, point.y)
        ctx.lineTo(other.x, other.y)
        ctx.stroke()
      }
    }
  }
}

function drawPulses(ctx: CanvasRenderingContext2D, points: ReturnType<typeof project>[]) {
  ctx.save()
  ctx.globalCompositeOperation = 'lighter'
  for (const pulse of pulses) {
    const from = points[pulse.from]
    const to = points[pulse.to]
    if (!from || !to) continue

    pulse.progress += reduceMotion ? 0 : pulse.speed
    if (pulse.progress >= 1) {
      pulse.progress = 0
      pulse.from = Math.floor(Math.random() * points.length)
      pulse.to = Math.floor(Math.random() * points.length)
    }

    const x = from.x + (to.x - from.x) * pulse.progress
    const y = from.y + (to.y - from.y) * pulse.progress
    const glow = ctx.createRadialGradient(x, y, 0, x, y, 16)
    glow.addColorStop(0, 'rgba(255,255,255,0.92)')
    glow.addColorStop(0.32, 'rgba(80,255,231,0.72)')
    glow.addColorStop(1, 'rgba(80,255,231,0)')
    ctx.fillStyle = glow
    ctx.beginPath()
    ctx.arc(x, y, 16, 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.restore()
}

function updateInteractiveNodes(points: ReturnType<typeof project>[]) {
  pointerX += (targetPointerX - pointerX) * 0.16
  pointerY += (targetPointerY - pointerY) * 0.16

  nodes.forEach((node, index) => {
    if (draggedNodeIndex === index) {
      node.offsetX = 0
      node.offsetY = 0
      return
    }

    node.dragX = null
    node.dragY = null
    node.x += (node.baseX - node.x) * 0.025
    node.y += (node.baseY - node.y) * 0.025
    node.z += (node.baseZ - node.z) * 0.025

    if (!pointerActive || reduceMotion) {
      node.offsetX *= 0.9
      node.offsetY *= 0.9
      return
    }

    const point = points[index]
    if (!point) return
    const dx = point.x - pointerX
    const dy = point.y - pointerY
    const distance = Math.hypot(dx, dy)
    const field = 170

    if (distance > 0 && distance < field) {
      const force = (1 - distance / field) * 0.036
      node.offsetX += (dx / distance) * force
      node.offsetY += (dy / distance) * force
      node.offsetX = Math.max(-0.2, Math.min(0.2, node.offsetX))
      node.offsetY = Math.max(-0.2, Math.min(0.2, node.offsetY))
    } else {
      node.offsetX *= 0.94
      node.offsetY *= 0.94
    }
  })
}

function drawNodes(ctx: CanvasRenderingContext2D, points: ReturnType<typeof project>[]) {
  ctx.save()
  ctx.globalCompositeOperation = 'lighter'
  points.forEach((point, index) => {
    const node = nodes[index]
    const glow = ctx.createRadialGradient(point.x, point.y, 0, point.x, point.y, point.radius * 5.5)
    glow.addColorStop(0, `hsla(${node.hue}, 100%, 78%, ${point.alpha})`)
    glow.addColorStop(0.4, `hsla(${node.hue}, 100%, 56%, ${point.alpha * 0.36})`)
    glow.addColorStop(1, `hsla(${node.hue}, 100%, 42%, 0)`)
    ctx.fillStyle = glow
    ctx.beginPath()
    ctx.arc(point.x, point.y, point.radius * 5.5, 0, Math.PI * 2)
    ctx.fill()

    ctx.fillStyle = `rgba(230, 255, 252, ${Math.min(1, point.alpha + 0.18)})`
    ctx.beginPath()
    ctx.arc(point.x, point.y, Math.max(1.2, point.radius * 0.74), 0, Math.PI * 2)
    ctx.fill()

    if (draggedNodeIndex === index) {
      ctx.strokeStyle = 'rgba(255, 255, 255, 0.78)'
      ctx.lineWidth = 1.5
      ctx.beginPath()
      ctx.arc(point.x, point.y, point.radius * 8.5, 0, Math.PI * 2)
      ctx.stroke()
    }
  })
  ctx.restore()
}

function drawRings(ctx: CanvasRenderingContext2D, time: number) {
  ctx.save()
  ctx.translate(centerX, centerY)
  ctx.globalCompositeOperation = 'lighter'
  const base = Math.min(width, height) * 0.26

  for (let i = 0; i < 5; i += 1) {
    ctx.rotate((time * 0.00012 + i * 0.22) * (i % 2 === 0 ? 1 : -1))
    ctx.strokeStyle = `rgba(${i % 2 ? '99, 102, 241' : '45, 255, 229'}, ${0.18 - i * 0.02})`
    ctx.lineWidth = 1.2
    ctx.setLineDash([26 + i * 4, 18 + i * 3])
    ctx.beginPath()
    ctx.ellipse(0, 0, base + i * 18, base * (0.38 + i * 0.045), i * 0.46, 0, Math.PI * 2)
    ctx.stroke()
  }

  ctx.restore()
}

function drawFrame(ctx: CanvasRenderingContext2D, time: number) {
  drawBackground(ctx, time)
  const interactivePoints = nodes.map((node) => project(node, time))
  lastProjectedPoints = interactivePoints
  updateInteractiveNodes(interactivePoints)
  const points = nodes.map((node) => project(node, time)).sort((a, b) => a.z - b.z)
  drawRings(ctx, time)
  drawConnections(ctx, points)
  drawPulses(ctx, points)
  drawNodes(ctx, points)
}

function pickNode(clientX: number, clientY: number) {
  let nearest = -1
  let nearestDistance = 28

  lastProjectedPoints.forEach((point, index) => {
    const distance = Math.hypot(point.x - clientX, point.y - clientY)
    if (distance < nearestDistance) {
      nearest = index
      nearestDistance = distance
    }
  })

  return nearest
}

function setDraggedNodePosition(index: number, clientX: number, clientY: number) {
  const scaleBase = Math.min(width, height) * 0.25
  const node = nodes[index]
  if (!node || scaleBase <= 0) return

  node.dragX = Math.max(-1.25, Math.min(1.25, (clientX - centerX) / scaleBase))
  node.dragY = Math.max(-1.25, Math.min(1.25, (clientY - centerY) / (scaleBase * 0.72)))
  node.x = node.dragX
  node.y = node.dragY
}

function updateHoveredLogo(clientX: number, clientY: number) {
  const target = document.elementFromPoint(clientX, clientY)
  const orbit = target instanceof Element ? target.closest<HTMLElement>('.jarvis-logo-orbit') : null
  if (orbit) {
    const rect = orbit.getBoundingClientRect()
    const chatWidth = 320
    const gap = 18
    const proposedX = rect.left < window.innerWidth / 2 ? rect.right + gap : rect.left - chatWidth - gap
    hoveredLogoName.value = orbit.dataset.logo ?? null
    chatPosition.value = {
      x: Math.max(18, Math.min(window.innerWidth - chatWidth - 18, proposedX)),
      y: Math.max(76, Math.min(window.innerHeight - 220, rect.top + rect.height / 2 - 86))
    }
    return
  }
  hoveredLogoName.value = null
}

function scheduleHoveredLogoUpdate(clientX: number, clientY: number) {
  pendingHoverPoint = { x: clientX, y: clientY }
  if (hoverFrame) return

  hoverFrame = window.requestAnimationFrame(() => {
    hoverFrame = 0
    if (!pendingHoverPoint) return
    updateHoveredLogo(pendingHoverPoint.x, pendingHoverPoint.y)
    pendingHoverPoint = null
  })
}

function stopAnimation() {
  if (!animationFrame) return
  window.cancelAnimationFrame(animationFrame)
  animationFrame = 0
}

function animate(ctx: CanvasRenderingContext2D) {
  if (animationFrame) return

  const tick = (time: number) => {
    if (!lastRenderTime || time - lastRenderTime >= targetFrameInterval) {
      drawFrame(ctx, time)
      lastRenderTime = time
    }
    animationFrame = window.requestAnimationFrame(tick)
  }
  animationFrame = window.requestAnimationFrame(tick)
}

onMounted(() => {
  syncThemeState()
  themeObserver = new MutationObserver(syncThemeState)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  telemetryTimer = window.setInterval(updateTelemetry, 2200)

  const canvas = canvasRef.value
  const ctx = canvas?.getContext('2d')
  if (!canvas || !ctx) return

  reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  configurePerformanceProfile()
  createNodes()
  resizeCanvas(canvas, ctx)
  animate(ctx)

  resizeHandler = () => {
    const previousMode = performanceMode
    configurePerformanceProfile()
    resizeCanvas(canvas, ctx)
    if (previousMode !== performanceMode) {
      createNodes()
    }
  }
  window.addEventListener('resize', resizeHandler)

  visibilityHandler = () => {
    if (document.hidden) {
      stopAnimation()
      return
    }
    lastRenderTime = 0
    animate(ctx)
  }
  document.addEventListener('visibilitychange', visibilityHandler)

  pointerMoveHandler = (event: PointerEvent) => {
    targetPointerX = event.clientX
    targetPointerY = event.clientY
    pointerActive = true
    scheduleHoveredLogoUpdate(event.clientX, event.clientY)
    if (draggedNodeIndex !== null) {
      setDraggedNodePosition(draggedNodeIndex, event.clientX, event.clientY)
    }
  }

  pointerDownHandler = (event: PointerEvent) => {
    targetPointerX = event.clientX
    targetPointerY = event.clientY
    pointerActive = true
    if (event.target instanceof Element && event.target.closest('.jarvis-logo-orbit')) {
      return
    }
    const picked = pickNode(event.clientX, event.clientY)
    if (picked >= 0) {
      draggedNodeIndex = picked
      setDraggedNodePosition(picked, event.clientX, event.clientY)
    }
  }

  pointerUpHandler = () => {
    if (draggedNodeIndex !== null) {
      nodes[draggedNodeIndex].dragX = null
      nodes[draggedNodeIndex].dragY = null
    }
    draggedNodeIndex = null
  }

  pointerLeaveHandler = () => {
    hoveredLogoName.value = null
  }

  window.addEventListener('pointermove', pointerMoveHandler)
  window.addEventListener('pointerdown', pointerDownHandler)
  window.addEventListener('pointerup', pointerUpHandler)
  window.addEventListener('pointercancel', pointerUpHandler)
  window.addEventListener('pointerleave', pointerLeaveHandler)
  window.addEventListener('blur', pointerUpHandler)
  window.addEventListener('blur', pointerLeaveHandler)
})

onBeforeUnmount(() => {
  if (telemetryTimer !== null) {
    window.clearInterval(telemetryTimer)
    telemetryTimer = null
  }
  if (themeObserver) {
    themeObserver.disconnect()
    themeObserver = null
  }
  stopAnimation()
  if (hoverFrame) {
    window.cancelAnimationFrame(hoverFrame)
    hoverFrame = 0
  }
  if (resizeHandler) {
    window.removeEventListener('resize', resizeHandler)
    resizeHandler = null
  }
  if (visibilityHandler) {
    document.removeEventListener('visibilitychange', visibilityHandler)
    visibilityHandler = null
  }
  if (pointerMoveHandler) {
    window.removeEventListener('pointermove', pointerMoveHandler)
    pointerMoveHandler = null
  }
  if (pointerDownHandler) {
    window.removeEventListener('pointerdown', pointerDownHandler)
    pointerDownHandler = null
  }
  if (pointerUpHandler) {
    window.removeEventListener('pointerup', pointerUpHandler)
    window.removeEventListener('pointercancel', pointerUpHandler)
    window.removeEventListener('blur', pointerUpHandler)
    pointerUpHandler = null
  }
  if (pointerLeaveHandler) {
    window.removeEventListener('pointerleave', pointerLeaveHandler)
    window.removeEventListener('blur', pointerLeaveHandler)
    pointerLeaveHandler = null
  }
  stopTypewriter()
})
</script>

<style scoped>
.jarvis-page {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  background: #f7f7f5;
  color: #e9fffb;
  cursor: crosshair;
  user-select: none;
  font-family:
    Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

.jarvis-page.is-dark {
  background: #020617;
}

.jarvis-canvas,
.jarvis-vignette,
.jarvis-grid {
  position: fixed;
  inset: 0;
}

.jarvis-canvas {
  z-index: 0;
}

.jarvis-vignette {
  z-index: 1;
  pointer-events: none;
  background:
    radial-gradient(circle at 50% 48%, transparent 0 28%, rgba(2, 6, 17, 0.12) 54%, rgba(247, 247, 245, 0.82) 100%),
    linear-gradient(90deg, rgba(247, 247, 245, 0.96), rgba(247, 247, 245, 0.08) 30%, rgba(247, 247, 245, 0.08) 70%, rgba(247, 247, 245, 0.96));
}

.jarvis-page.is-dark .jarvis-vignette {
  background:
    radial-gradient(circle at 50% 48%, transparent 0 34%, rgba(2, 6, 17, 0.18) 62%, rgba(2, 6, 23, 0.92) 100%),
    linear-gradient(90deg, rgba(2, 6, 23, 0.92), rgba(8, 24, 44, 0.22) 30%, rgba(8, 24, 44, 0.22) 70%, rgba(2, 6, 23, 0.92));
}

.jarvis-grid {
  z-index: 2;
  pointer-events: none;
  opacity: 0.34;
  background-image:
    linear-gradient(rgba(94, 255, 238, 0.13) 1px, transparent 1px),
    linear-gradient(90deg, rgba(94, 255, 238, 0.13) 1px, transparent 1px);
  background-size: 64px 64px;
  mask-image: radial-gradient(circle at 50% 48%, #000 0 46%, transparent 76%);
}

.jarvis-stage {
  position: relative;
  z-index: 3;
  min-height: 100vh;
  padding: 28px;
}

.jarvis-topbar,
.jarvis-bottom-hud {
  position: absolute;
  left: 28px;
  right: 28px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
}

.jarvis-topbar {
  top: 22px;
}

.jarvis-brand,
.jarvis-status,
.jarvis-panel,
.jarvis-bottom-hud {
  border: 1px solid rgba(92, 255, 236, 0.28);
  background: rgba(4, 14, 28, 0.62);
  box-shadow: 0 0 30px rgba(45, 255, 229, 0.1), inset 0 0 24px rgba(45, 255, 229, 0.04);
  backdrop-filter: blur(16px);
}

.jarvis-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  padding: 10px 14px;
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
}

.jarvis-brand-mark {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border: 1px solid rgba(106, 255, 240, 0.62);
  color: #79fff0;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
  box-shadow: inset 0 0 18px rgba(45, 255, 229, 0.28);
}

.jarvis-brand strong,
.jarvis-brand em {
  display: block;
  line-height: 1.1;
}

.jarvis-brand strong {
  font-size: 15px;
}

.jarvis-brand em {
  margin-top: 4px;
  color: rgba(212, 255, 249, 0.62);
  font-size: 11px;
  font-style: normal;
}

.jarvis-status {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  color: rgba(218, 255, 251, 0.82);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
}

.jarvis-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #55ffe9;
  box-shadow: 0 0 16px #55ffe9;
  animation: jarvis-ping 1.3s ease-in-out infinite;
}

.jarvis-core {
  position: absolute;
  top: 50%;
  left: 50%;
  width: min(38vw, 500px);
  aspect-ratio: 1;
  transform: translate(-50%, -52%);
  pointer-events: auto;
  animation: jarvis-core-float 5.4s ease-in-out infinite;
}

.jarvis-ring,
.jarvis-brain,
.jarvis-orbit,
.jarvis-energy-wave {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  pointer-events: none;
}

.jarvis-ring {
  border: 1px solid rgba(70, 255, 233, 0.24);
}

.jarvis-ring-a {
  border-color: rgba(70, 255, 233, 0.36);
  animation: jarvis-spin 7.5s linear infinite;
  box-shadow: inset 0 0 40px rgba(64, 255, 231, 0.06), 0 0 36px rgba(64, 255, 231, 0.1);
}

.jarvis-ring-b {
  inset: 8%;
  border-style: dashed;
  border-color: rgba(122, 116, 255, 0.34);
  animation: jarvis-spin 10.5s linear infinite reverse;
}

.jarvis-ring-c {
  inset: 18%;
  border-color: rgba(255, 255, 255, 0.18);
  animation: jarvis-breathe 3.6s ease-in-out infinite;
}

.jarvis-orbit {
  border: 1px solid rgba(95, 255, 238, 0.18);
  filter: drop-shadow(0 0 18px rgba(45, 255, 229, 0.22));
}

.jarvis-orbit-one {
  inset: 5%;
  transform: rotateX(62deg) rotateZ(12deg);
  animation: jarvis-orbit-spin 5.8s linear infinite;
}

.jarvis-orbit-two {
  inset: 15%;
  transform: rotateX(68deg) rotateZ(-36deg);
  animation: jarvis-orbit-spin 8s linear infinite reverse;
}

.jarvis-orbit span {
  position: absolute;
  left: 50%;
  top: -5px;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: #eafffb;
  box-shadow:
    0 0 10px rgba(255, 255, 255, 0.92),
    0 0 24px rgba(45, 255, 229, 0.95),
    0 0 52px rgba(99, 102, 241, 0.62);
}

.jarvis-orbit span:nth-child(2) {
  top: auto;
  bottom: -5px;
  width: 8px;
  height: 8px;
  background: #a8b4ff;
}

.jarvis-orbit-two span:nth-child(2) {
  left: auto;
  right: 10%;
  top: 18%;
}

.jarvis-orbit-two span:nth-child(3) {
  left: 18%;
  top: auto;
  bottom: 9%;
  width: 7px;
  height: 7px;
}

.jarvis-logo-system {
  position: absolute;
  inset: -5%;
  z-index: 4;
  pointer-events: none;
  transform-style: preserve-3d;
}

.jarvis-logo-system::before {
  content: "";
  position: absolute;
  inset: 16%;
  border-radius: 50%;
  border: 1px solid rgba(235, 255, 252, 0.12);
  transform: rotateX(66deg) rotateZ(-12deg);
  box-shadow: 0 0 34px rgba(45, 255, 229, 0.08);
}

.jarvis-logo-orbit {
  position: absolute;
  top: 50%;
  left: 50%;
  display: grid;
  place-items: center;
  width: 76px;
  height: 76px;
  margin: -38px 0 0 -38px;
  opacity: 1;
  pointer-events: auto;
  transform:
    rotate(var(--angle))
    translateX(var(--radius))
    rotate(calc(var(--angle) * -1))
    rotate(var(--tilt));
  animation: jarvis-model-orbit var(--duration) linear infinite;
  animation-delay: var(--delay);
  will-change: transform;
}

.jarvis-logo-orbit:hover,
.jarvis-logo-orbit.is-hovered {
  z-index: 30;
  animation-play-state: paused;
}

.jarvis-logo-orbit-inner {
  z-index: 6;
}

.jarvis-logo-orbit-middle {
  z-index: 5;
}

.jarvis-logo-orbit-outer {
  z-index: 4;
}

.jarvis-model-logo {
  position: relative;
  display: grid;
  place-items: center;
  width: 52px;
  height: 52px;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--brand) 64%, white 20%);
  border-radius: 50%;
  background:
    radial-gradient(circle at 35% 26%, rgba(255, 255, 255, 0.42), rgba(255, 255, 255, 0.09) 28%, transparent 52%),
    linear-gradient(145deg, rgba(255, 255, 255, 0.38), rgba(236, 246, 255, 0.12));
  color: var(--brand);
  backdrop-filter: blur(3px);
  box-shadow:
    0 0 12px var(--brand-soft),
    0 0 28px rgba(45, 255, 229, 0.08),
    inset 0 0 16px rgba(255, 255, 255, 0.16);
  cursor: pointer;
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    border-color 180ms ease,
    background 180ms ease;
  will-change: transform;
}

.jarvis-model-logo::before {
  content: "";
  position: absolute;
  inset: 5px;
  border-radius: inherit;
  border: 1px solid rgba(2, 6, 17, 0.08);
}

.jarvis-model-logo:hover,
.jarvis-logo-orbit.is-hovered .jarvis-model-logo {
  border-color: color-mix(in srgb, var(--brand) 72%, white 28%);
  background:
    radial-gradient(circle at 35% 26%, rgba(255, 255, 255, 0.64), rgba(255, 255, 255, 0.18) 28%, transparent 52%),
    linear-gradient(145deg, rgba(255, 255, 255, 0.56), rgba(236, 246, 255, 0.2));
  box-shadow:
    0 0 18px var(--brand-soft),
    0 0 44px rgba(45, 255, 229, 0.18),
    inset 0 0 18px rgba(255, 255, 255, 0.22);
  transform: scale(1.34);
}

.jarvis-model-logo svg {
  position: relative;
  width: 31px;
  height: 31px;
  opacity: 1;
  filter:
    drop-shadow(0 0 1px color-mix(in srgb, var(--brand) 72%, white 28%))
    drop-shadow(0 0 4px var(--brand-soft));
  shape-rendering: geometricPrecision;
}

.jarvis-model-logo svg path {
  stroke: color-mix(in srgb, var(--brand) 82%, white 18%);
  stroke-width: 0.18px;
  paint-order: stroke fill;
  vector-effect: non-scaling-stroke;
}

.jarvis-model-logo-text span {
  position: relative;
  max-width: 38px;
  overflow: hidden;
  font-family:
    Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 14.5px;
  font-weight: 950;
  letter-spacing: 0;
  line-height: 1;
  opacity: 1;
  text-align: center;
  text-overflow: ellipsis;
  text-rendering: geometricPrecision;
  text-shadow:
    0 0 1px color-mix(in srgb, var(--brand) 75%, white 25%),
    0 0 4px var(--brand-soft);
  -webkit-font-smoothing: antialiased;
  white-space: nowrap;
}

.jarvis-model-chat {
  position: fixed;
  left: var(--chat-x);
  top: var(--chat-y);
  z-index: 35;
  width: min(320px, calc(100vw - 36px));
  min-height: 166px;
  padding: 16px;
  pointer-events: none;
  border: 1px solid color-mix(in srgb, var(--brand) 58%, rgba(92, 255, 236, 0.34));
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--brand) 16%, rgba(4, 14, 28, 0.78)), rgba(4, 14, 28, 0.72)),
    radial-gradient(circle at 18% 0%, var(--brand-soft), transparent 48%);
  box-shadow:
    0 0 28px var(--brand-soft),
    0 0 58px rgba(45, 255, 229, 0.1),
    inset 0 0 24px rgba(255, 255, 255, 0.05);
  backdrop-filter: blur(18px);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
}

.jarvis-model-chat::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.16), transparent),
    repeating-linear-gradient(180deg, rgba(255, 255, 255, 0.05) 0 1px, transparent 1px 7px);
  mix-blend-mode: screen;
  opacity: 0.34;
}

.jarvis-chat-head,
.jarvis-chat-foot {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.jarvis-chat-head {
  justify-content: flex-start;
}

.jarvis-chat-head strong,
.jarvis-chat-head em {
  display: block;
  line-height: 1.1;
}

.jarvis-chat-head strong {
  color: #f5fffd;
  font-size: 15px;
  font-weight: 850;
}

.jarvis-chat-head em {
  margin-top: 4px;
  color: rgba(224, 255, 251, 0.62);
  font-size: 11px;
  font-style: normal;
}

.jarvis-model-chat p {
  position: relative;
  min-height: 70px;
  margin: 14px 0 12px;
  color: rgba(238, 255, 252, 0.9);
  font-size: 14px;
  font-weight: 650;
  line-height: 1.62;
  text-shadow: 0 0 12px rgba(45, 255, 229, 0.18);
}

.jarvis-type-cursor {
  display: inline-block;
  width: 7px;
  height: 1.1em;
  margin-left: 3px;
  vertical-align: -0.18em;
  background: color-mix(in srgb, var(--brand) 72%, white 28%);
  box-shadow: 0 0 10px var(--brand-soft);
  animation: jarvis-cursor-blink 0.72s steps(2, start) infinite;
}

.jarvis-chat-foot {
  color: rgba(222, 255, 251, 0.58);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
}

.jarvis-chat-foot span:first-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.jarvis-energy-wave {
  inset: 9%;
  border: 1px solid transparent;
  background:
    conic-gradient(from 90deg, transparent 0deg, rgba(81, 255, 236, 0.95) 18deg, transparent 44deg, transparent 138deg, rgba(127, 118, 255, 0.72) 160deg, transparent 192deg, transparent 360deg)
    border-box;
  mask: linear-gradient(#000 0 0) padding-box, linear-gradient(#000 0 0);
  mask-composite: exclude;
  opacity: 0.64;
  animation: jarvis-energy-spin 3.2s linear infinite;
}

.jarvis-energy-wave-b {
  inset: 24%;
  opacity: 0.5;
  animation-duration: 2.4s;
  animation-direction: reverse;
}

.jarvis-brain {
  inset: 30%;
  display: grid;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgba(133, 255, 243, 0.48);
  background:
    linear-gradient(135deg, rgba(64, 255, 231, 0.14), rgba(114, 99, 255, 0.1)),
    radial-gradient(circle at 50% 42%, rgba(245, 255, 253, 0.24), rgba(34, 255, 229, 0.07) 42%, rgba(4, 14, 28, 0.78) 100%);
  box-shadow:
    0 0 40px rgba(62, 255, 232, 0.22),
    0 0 110px rgba(97, 92, 255, 0.18),
    inset 0 0 44px rgba(255, 255, 255, 0.08);
  animation: jarvis-brain-pulse 2.2s ease-in-out infinite;
}

.jarvis-brain::before,
.jarvis-brain::after {
  content: "";
  position: absolute;
  inset: 14%;
  border-radius: 50%;
  border: 1px solid rgba(230, 255, 252, 0.22);
}

.jarvis-brain::after {
  inset: 26%;
  border-color: rgba(92, 255, 236, 0.38);
  transform: rotate(45deg);
}

.jarvis-brain-scan {
  position: absolute;
  inset: -30%;
  background: linear-gradient(transparent 42%, rgba(255, 255, 255, 0.42), transparent 58%);
  animation: jarvis-scan 1.7s ease-in-out infinite;
}

.jarvis-brain-label {
  position: relative;
  color: #f5fffd;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: clamp(18px, 3vw, 38px);
  font-weight: 800;
  text-shadow: 0 0 18px rgba(94, 255, 238, 0.9);
}

.jarvis-copy {
  position: absolute;
  top: 50%;
  left: 5vw;
  max-width: 330px;
  transform: translateY(-46%);
}

.jarvis-kicker {
  margin: 0 0 14px;
  color: #69fff0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
}

.jarvis-copy h1 {
  margin: 0;
  max-width: 10ch;
  font-size: clamp(42px, 6vw, 78px);
  font-weight: 800;
  line-height: 0.92;
}

.jarvis-copy p:last-child {
  margin: 22px 0 0;
  color: rgba(224, 255, 251, 0.72);
  font-size: 16px;
  line-height: 1.7;
}

.jarvis-panel {
  position: absolute;
  width: min(24vw, 300px);
  padding: 16px;
  clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px));
}

.jarvis-panel-left {
  right: 5vw;
  top: 25%;
}

.jarvis-panel-right {
  right: 7vw;
  bottom: 20%;
}

.jarvis-panel-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: rgba(221, 255, 251, 0.74);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
}

.jarvis-panel-heading b {
  color: #69fff0;
  font-size: 13px;
  transition: color 0.2s ease;
}

.jarvis-panel ul {
  display: grid;
  gap: 10px;
  margin: 16px 0 0;
  padding: 0;
  list-style: none;
}

.jarvis-panel li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border-top: 1px solid rgba(92, 255, 236, 0.14);
  padding-top: 10px;
  color: rgba(226, 255, 252, 0.68);
  font-size: 13px;
}

.jarvis-panel li strong {
  color: #f6fffd;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  transition: color 0.2s ease;
}

.jarvis-bars {
  display: flex;
  align-items: end;
  gap: 8px;
  height: 92px;
  margin-top: 18px;
}

.jarvis-bars span {
  flex: 1;
  height: var(--h);
  min-height: 18px;
  border: 1px solid rgba(106, 255, 240, 0.32);
  background: linear-gradient(180deg, rgba(99, 102, 241, 0.75), rgba(45, 255, 229, 0.78));
  box-shadow: 0 0 18px rgba(45, 255, 229, 0.18);
  transition: height 0.9s ease;
  animation: jarvis-bars 1.8s ease-in-out infinite alternate;
}

.jarvis-bars span:nth-child(2n) {
  animation-delay: -0.7s;
}

.jarvis-panel code {
  display: block;
  margin-top: 14px;
  overflow: hidden;
  color: rgba(223, 255, 251, 0.72);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.jarvis-bottom-hud {
  bottom: 22px;
  flex-wrap: wrap;
  justify-content: center;
  padding: 12px 16px;
  color: rgba(221, 255, 251, 0.72);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
}

.jarvis-bottom-hud span {
  white-space: nowrap;
}

@keyframes jarvis-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes jarvis-core-float {
  0%,
  100% {
    translate: 0 0;
    filter: brightness(1);
  }
  50% {
    translate: 0 -14px;
    filter: brightness(1.2);
  }
}

@keyframes jarvis-orbit-spin {
  to {
    rotate: 360deg;
  }
}

@keyframes jarvis-model-orbit {
  from {
    transform:
      rotate(var(--angle))
      translateX(var(--radius))
      rotate(calc(var(--angle) * -1))
      rotate(var(--tilt));
  }
  to {
    transform:
      rotate(calc(var(--angle) + 360deg))
      translateX(var(--radius))
      rotate(calc(var(--angle) * -1 - 360deg))
      rotate(var(--tilt));
  }
}

@keyframes jarvis-energy-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes jarvis-brain-pulse {
  0%,
  100% {
    transform: scale(0.98);
    box-shadow:
      0 0 40px rgba(62, 255, 232, 0.2),
      0 0 110px rgba(97, 92, 255, 0.16),
      inset 0 0 44px rgba(255, 255, 255, 0.08);
  }
  50% {
    transform: scale(1.05);
    box-shadow:
      0 0 66px rgba(62, 255, 232, 0.42),
      0 0 160px rgba(97, 92, 255, 0.32),
      inset 0 0 60px rgba(255, 255, 255, 0.16);
  }
}

@keyframes jarvis-breathe {
  50% {
    transform: scale(0.92);
    opacity: 0.68;
  }
}

@keyframes jarvis-scan {
  0%,
  100% {
    transform: translateY(-36%) rotate(18deg);
  }
  50% {
    transform: translateY(36%) rotate(18deg);
  }
}

@keyframes jarvis-ping {
  50% {
    opacity: 0.45;
    transform: scale(1.8);
  }
}

@keyframes jarvis-bars {
  to {
    filter: brightness(1.34);
    transform: scaleY(0.78);
  }
}

@keyframes jarvis-cursor-blink {
  50% {
    opacity: 0;
  }
}

.jarvis-chat-enter-active,
.jarvis-chat-leave-active {
  transition:
    opacity 150ms ease,
    transform 150ms ease;
}

.jarvis-chat-enter-from,
.jarvis-chat-leave-to {
  opacity: 0;
  transform: translateY(8px) scale(0.96);
}

@media (max-width: 920px) {
  .jarvis-stage {
    min-height: 100svh;
    padding: 18px;
  }

  .jarvis-topbar,
  .jarvis-bottom-hud {
    left: 18px;
    right: 18px;
  }

  .jarvis-status {
    display: none;
  }

  .jarvis-core {
    top: 43%;
    width: min(92vw, 520px);
    opacity: 0.92;
  }

  .jarvis-logo-orbit {
    width: 64px;
    height: 64px;
    margin: -32px 0 0 -32px;
  }

  .jarvis-model-logo {
    width: 42px;
    height: 42px;
  }

  .jarvis-model-logo svg {
    width: 23px;
    height: 23px;
  }

  .jarvis-model-logo-text span {
    max-width: 31px;
    font-size: 11px;
  }

  .jarvis-model-chat {
    width: min(300px, calc(100vw - 28px));
    min-height: 154px;
    padding: 14px;
  }

  .jarvis-model-chat p {
    min-height: 64px;
    font-size: 13px;
  }

  .jarvis-copy {
    top: auto;
    bottom: 120px;
    left: 18px;
    right: 18px;
    max-width: 520px;
    transform: none;
  }

  .jarvis-copy h1 {
    max-width: 11ch;
    font-size: clamp(42px, 14vw, 72px);
  }

  .jarvis-copy p:last-child {
    max-width: 420px;
    font-size: 14px;
  }

  .jarvis-panel {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .jarvis-dot,
  .jarvis-ring,
  .jarvis-core,
  .jarvis-orbit,
  .jarvis-energy-wave,
  .jarvis-brain,
  .jarvis-brain-scan,
  .jarvis-bars span {
    animation: none;
  }
}
</style>
