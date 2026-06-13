# TOP-AI Model Catalog Frontend Plan

## Goal

Build a model catalog page that helps users understand which models TOP-AI currently sells, what they are good at, and how they are priced.

This page is not a global model encyclopedia. It is a product shelf based on models that are already available through our configured upstream channels and sellable pricing rules.

## Current Project Structure

The repository already has a documentation directory:

- `docs/`
- `DEV_GUIDE.md`

Frontend code should continue following the existing layout:

- Public pages: `frontend/src/views/public/`
- User pages: `frontend/src/views/user/`
- Feature components: `frontend/src/components/<feature>/`
- API wrappers: `frontend/src/api/`
- Router config: `frontend/src/router/index.ts`
- I18n copy: `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`
- Shared pricing format helpers: `frontend/src/utils/pricing.ts`

## Architecture And Placement Rules

Code must follow the existing Sub2API project structure. Do not pile feature code into unrelated files or random directories.

Hard rules:

- Do not place model catalog components directly inside `HomeView.vue`.
- Do not place reusable model catalog components under `frontend/src/views/`.
- Do not place page-level code under `frontend/src/components/`.
- Do not put API request code inside Vue components.
- Do not put data transformation or filtering logic inside templates.
- Do not create duplicate API clients.
- Do not modify existing API wrappers unless the existing endpoint contract actually changes.
- Do not change existing authenticated user APIs for the public catalog.
- Do not mix admin-only data structures into public frontend components.
- Do not add backend route logic inside unrelated handlers.
- Do not reuse admin route names, admin components, or admin DTOs directly for public display.
- Do not hard-code catalog data in Vue files.

Required placement:

- Public route page: `frontend/src/views/public/ModelCatalogView.vue`
- Public catalog API wrapper: `frontend/src/api/publicModels.ts`
- Catalog data transformation/filtering: `frontend/src/utils/modelCatalog.ts`
- Catalog UI components: `frontend/src/components/models/`
- Catalog component tests: `frontend/src/components/models/__tests__/`
- Catalog utility tests: `frontend/src/utils/__tests__/modelCatalog.spec.ts`
- Route registration only: `frontend/src/router/index.ts`
- Chinese copy only: `frontend/src/i18n/locales/zh.ts`
- English copy only: `frontend/src/i18n/locales/en.ts`

API placement rules:

- Public frontend calls must go through `frontend/src/api/publicModels.ts`.
- `publicModels.ts` should use the existing `apiClient` pattern from `frontend/src/api/`.
- The public catalog frontend must call only the public catalog endpoint.
- The public catalog frontend must not call `/api/v1/channels/available` or `/api/v1/groups/rates`.
- If backend implementation is added, create a dedicated public catalog handler/service path that returns a strict public DTO.
- Public API DTOs must be separate from admin DTOs and must contain only fields safe for public display.

The intended dependency direction is:

```text
ModelCatalogView.vue
  -> components/models/*
  -> utils/modelCatalog.ts
  -> api/publicModels.ts
  -> GET /api/v1/public/models/catalog
```

Anything outside this flow needs an explicit reason before implementation.

## Scope

The first version should be a public, no-login model catalog.

The catalog must show real sellable TOP-AI models, not static demo data. Because the existing user available channels endpoint is authenticated, a public no-login catalog needs a read-only, white-listed public data source.

Preferred public endpoint:

- `GET /api/v1/public/models/catalog`

This endpoint should be backed by existing channel pricing and supported-model data, but it must return only public display fields.

The existing authenticated data path remains useful as an implementation reference:

- authenticated endpoint: `GET /api/v1/channels/available`
- existing frontend wrapper: `frontend/src/api/channels.ts`
- existing page using similar data: `frontend/src/views/user/AvailableChannelsView.vue`

Non-goals for the first version:

- No login requirement.
- No admin setting redesign.
- No backend billing logic changes.
- No model test calls.
- No real-time upstream probing.
- No hard-coded catalog data.
- No hard-coded platform list.
- No multi-currency support. The project settles in USD only.

## Route

Add a public route:

- `/models`
- route name: `ModelCatalog`
- component: `@/views/public/ModelCatalogView.vue`
- auth: not required
- sidebar/menu title: Model Catalog / 模型广场

Placement:

- Add a "模型广场 / Models" entry on the home page top action area.
- Add a home-page CTA near the existing docs/login actions if space allows.
- Optionally add a logged-in sidebar entry later, but the primary entry is public `/models`.
- Keep `/pricing` available as a future alias only if product naming later prefers "pricing".

## Files To Add

Frontend page:

- `frontend/src/views/public/ModelCatalogView.vue`

Feature components:

- `frontend/src/components/models/ModelCatalogHeader.vue`
- `frontend/src/components/models/ModelCatalogFilters.vue`
- `frontend/src/components/models/ModelCard.vue`
- `frontend/src/components/models/ModelPriceSummary.vue`
- `frontend/src/components/models/ModelCapabilityTags.vue`

Feature helpers:

- `frontend/src/utils/modelCatalog.ts`

Optional tests:

- `frontend/src/components/models/__tests__/ModelCard.spec.ts`
- `frontend/src/utils/__tests__/modelCatalog.spec.ts`

## Files To Modify

- `frontend/src/router/index.ts`
  - Register `/models`.

- `frontend/src/i18n/locales/zh.ts`
  - Add Chinese copy for page title, filters, card labels, empty states.

- `frontend/src/i18n/locales/en.ts`
  - Add English copy for the same keys.

- Existing navigation layout if the app menu is configured there.
  - Add a menu entry only if the current project has a central nav definition.

## Data Source

Use a public, read-only catalog API:

- `publicModelsAPI.getCatalog()`
- suggested frontend wrapper: `frontend/src/api/publicModels.ts`

Suggested backend route:

- `GET /api/v1/public/models/catalog`

This public response should be derived from current upstream/channel/model pricing configuration.

Do not call authenticated user metadata endpoints on the public page:

- `userChannelsAPI.getAvailable()`
- `userGroupsAPI.getUserGroupRates()`

Those existing APIs can still be used as a reference for DTO shape and UI behavior.

The page should transform public catalog rows into display cards.

Data availability rules:

- Render only models returned by the backend.
- Render only platforms present in the returned model list.
- Render only public sale price data returned by the public catalog API.
- If the public catalog is disabled or the backend returns an empty list, show an empty state instead of fallback fake data.
- Do not include user-exclusive groups or user-specific pricing in the public page.

Input shape:

- Platform
- Provider display name
- Model name
- Public sale price
- Billing mode
- Capability/display tags
- Sale status

Display shape:

```ts
interface ModelCatalogItem {
  id: string
  name: string
  provider: string
  platforms: string[]
  publicStatus: string
  pricing: {
    billingMode: string
    inputPrice: number | null
    outputPrice: number | null
    cacheWritePrice: number | null
    cacheReadPrice: number | null
    imageOutputPrice: number | null
    perRequestPrice: number | null
  } | null
  capabilities: string[]
  badges: string[]
}
```

## Card Rules

Do not show one card per channel row if the same model appears in multiple channels.

Group by:

- normalized model name
- provider/platform when needed

A model card should show:

- Model name
- Provider logo/icon
- Capability tags
- Price summary
- Available platforms
- Billing mode
- Context/capability hints if available from current data
- Copy model name button
- Use button that links to API key creation or documentation

Do not show:

- Upstream API keys
- Real upstream base URLs
- Internal account IDs
- Internal channel IDs
- Internal routing weights
- Full failure rates
- Account balance
- Proxy information
- User-exclusive group information

## Pricing Display

Pricing should be shown in a user-readable way:

- Token billing: input price / output price per 1M tokens
- Image billing: image output price
- Per request billing: per request price
- Missing pricing: show "Contact admin" or "Price unavailable"

For multiple channels with the same model:

- Show "from {price} / 1M tokens" or "lowest available price"
- Details can show all accessible groups/prices in a collapsed section later

Currency and numeric formatting:

- TOP-AI settles in USD only for the first version.
- Display prices in USD.
- Use a shared USD formatting helper instead of scattering `$` strings across templates.
- Do not add currency selectors or currency conversion UI.
- Use tabular numbers for price display.
- Keep raw values in transform helpers and format them only at the component boundary.

## Filters

First version filters must be generated from the returned backend data. Do not hard-code visible filter options.

- Search by model name.
- Provider/platform filters are derived from the platforms present in the current model list.
- Capability filters are derived from capabilities inferred from the current model list.
- Billing mode filters are derived from billing modes present in the current model list.
- "Only show models with price" is available only when at least one model has pricing.
- Sort options should apply only to the current actual model list.

Do not show a provider/platform option if no model from that provider/platform exists in the backend response.

Do not show a capability option if no model in the current list has that capability.

Do not show a billing mode option if no model in the current list uses that billing mode.

Recommended sort is allowed only if the frontend can derive a recommendation signal from current data, such as price, availability, or configured badges. If there is no reliable signal, omit the recommended sort option in the first version.

Capability tags can be inferred conservatively from model names, but only for models actually returned by the backend:

- `gpt`, `claude`, `gemini`, `deepseek`, `grok`
- `embedding`
- `image`
- `reasoning`
- `coding`
- `fast`
- `low-cost`
- `long-context`

Keep this inference in `frontend/src/utils/modelCatalog.ts`, not inside the Vue template.

The filter-building helper should return both catalog items and available filter facets:

```ts
interface ModelCatalogFacets {
  platforms: string[]
  capabilities: string[]
  billingModes: string[]
  hasPricedModels: boolean
  sortOptions: string[]
}
```

The UI renders only these returned facets.

## UX Direction

The page should look like a model marketplace, not an admin table.

It must match the current TOP-AI home style:

- Jarvis / neural gateway mood
- dark technical grid background
- cyan, teal, blue, and soft violet highlights
- glass panels with thin luminous borders
- sharp metal / command-center feeling
- restrained motion, not noisy animation
- premium, spacious, product-shelf layout

The reference pricing page can be used for the card-based display method, but the visual language should stay TOP-AI. Do not copy its brand theme, spacing, or color system directly.

Recommended layout:

- Page title: 模型广场 / Model Catalog
- Subtitle: TOP-AI 当前可售 AI 模型与美元价格
- Search and filters row
- Responsive card grid
- Empty state with link to documentation or contact/admin hint

Use cards because users understand cards as products.

Each card should answer:

- What is this model?
- What is it good at?
- How much does it cost?
- Can I use it now?
- What do I do next?

## Visual Design Rules

The model catalog should feel like a continuation of the home page, not a separate admin section.

Page shell:

- Use a dark command-center background by default.
- Reuse the home page visual direction: radial center glow, faint grid, teal/blue edge light, glass surfaces.
- Keep the page readable in light mode, but the premium look should be strongest in dark mode.
- Do not use plain white dashboard cards as the primary design.
- Do not use large marketing hero copy. The user is already inside the product.

Header:

- Compact product header, not a landing hero.
- Show "模型广场 / Model Catalog" as the functional title.
- Add a short subtitle explaining that these are TOP-AI sellable models and USD prices.
- Include small status chips derived from page behavior, such as "Public catalog", "USD pricing", "No model traffic".
- Do not show operational status claims unless the backend data supports them.

Cards:

- Cards should look like polished product tiles.
- Use 8px or less border radius unless matching an existing local component requires otherwise.
- Use glass background, subtle border, glow on hover, and a clear model logo area.
- Do not nest cards inside cards.
- Do not make every card the same color.
- Use provider accent colors lightly only for providers that actually appear in returned data.
- Example accents: OpenAI green, Claude orange, Gemini blue, DeepSeek blue, Qwen purple, Grok neutral.
- Provider color is an accent only; the page should still read as TOP-AI.

Each card should have this hierarchy:

1. Provider mark and model name.
2. Capability tags.
3. Price summary.
4. Availability and accessible groups.
5. Actions: copy model name, use/create key, view docs.

Model logos:

- Use the existing `PlatformIcon` / `ModelIcon` pattern if it fits.
- If a provider has no logo, use a text mark with the same visual treatment as the home page orbit logos.
- Keep logo sizes consistent across cards.
- Do not use blurry or low-contrast logo rendering.

Price display:

- Price should be the visual anchor of the card.
- Use monospace/tabular numbers for price.
- Token models should show input and output prices together.
- If multiple routes exist, show "from" pricing on the card and put details in an expandable area later.

Motion:

- Hover can lift the card slightly and strengthen the border glow.
- Avoid constant animation on every card.
- Background motion should be slow and low-cost.
- Respect reduced-motion settings.

Mobile:

- One card per row.
- No horizontal overflow.
- Price and action buttons must remain readable.
- Filters should collapse into a compact control row.

## Model Card Example Layout

This is a layout example only. It is not static fixture data.

```text
+--------------------------------------------------+
| [Provider logo]  Claude Sonnet 4.5        Stable |
| Long context  Reasoning  Writing  Coding         |
|                                                  |
| Input  {price} / 1M      Output  {price} / 1M    |
| Public catalog · USD pricing                     |
|                                                  |
| [Copy model]                       [Use model]   |
+--------------------------------------------------+
```

## Security

Frontend-only catalog does not consume AI traffic because it only renders existing metadata.

It must not call inference endpoints such as:

- `/v1/chat/completions`
- `/v1/responses`
- `/v1/messages`

It should only call existing metadata endpoints:

- `/api/v1/public/models/catalog`

The page must not expose:

- upstream API keys
- upstream base URLs
- internal channel IDs
- internal account IDs
- routing weights
- account balance
- proxy information
- detailed latency or failure metrics

Clicking "Use model" must navigate to a normal product flow, such as API key creation or documentation. It must not send a model request.

## Loading, Error, And Empty States

The page should define these states explicitly:

- Loading: show lightweight catalog skeleton cards.
- Empty: show that no public sellable models are currently available, or that the catalog is not enabled.
- Error: show a retry action and the existing project error toast behavior.
- No pricing: keep the model visible if it is available, but mark price as unavailable.

## Public API Field Whitelist

The public catalog API should return only:

- model name
- provider display name
- capability tags
- public USD price or starting price
- sale status

It should not return internal routing or upstream data.

## Implementation Order

1. Add `docs/MODEL_CATALOG_FRONTEND_PLAN.md`.
2. Add the public catalog API wrapper in `frontend/src/api/publicModels.ts`.
3. Add transform utilities in `frontend/src/utils/modelCatalog.ts`.
4. Add model catalog components under `frontend/src/components/models/`.
5. Add `frontend/src/views/public/ModelCatalogView.vue`.
6. Register `/models` in `frontend/src/router/index.ts` as a public route.
7. Add a home-page entry to `/models`.
8. Add i18n copy in Chinese and English.
9. Run frontend typecheck and targeted tests.
10. Open `/models` locally and verify card layout, filters, language switch, dark/light mode, and mobile width.

## Verification Checklist

- No unavailable platform appears in filters.
- No unavailable billing mode appears in filters.
- No unavailable capability appears in filters.
- No fake model appears when backend returns an empty list.
- Same model from multiple channels is grouped into one card.
- Price display is USD-only and uses shared formatting.
- The page does not call inference endpoints.
- The page does not call authenticated user-only endpoints.
- Dark and light modes remain readable.
- Chinese and English text switch correctly.
- Mobile layout has no horizontal overflow.

## Open Decisions

- Whether cards should show exact prices or starting prices.
- Whether capability tags should be manually configured later instead of inferred from model names.
