-- 删除 upstream_price_sources.platform 列
--
-- platform 字段原为纯展示用元数据标签，但实际不参与任何同步/解析/计费逻辑：
--   - 解析器选择只看 parser_type（ParserByType）
--   - 建议倍率的"平台匹配"用的是 inferPlatformFromModelName 从模型名推断的平台，
--     与本字段无关
--   - HTTP 拉取 / diff / 入库 / 告警均不读取该字段
-- 故移除以消除"填了平台会影响计费"的误导，并清理前后端三处互相不一致的枚举定义。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE upstream_price_sources DROP COLUMN IF EXISTS platform;
