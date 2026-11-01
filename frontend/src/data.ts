export type ProductKind = "ai" | "video" | "tool" | "game";
export type ProductArt =
  | "spark"
  | "sun"
  | "orbit"
  | "play"
  | "music"
  | "gem"
  | "book"
  | "game";

export interface Product {
  delivery_mode?: "auto" | "manual";
  id: number;
  category_id: number;
  name: string;
  description: string;
  price_cents: number;
  original_price_cents: number;
  stock: number;
  sales: number;
  cover?: string;
  redeem_guide?: string;
  kind: ProductKind;
  art: ProductArt;
  subtitle: string;
  badge?: string;
  tags: string[];
}

export interface Category {
  id: number;
  name: string;
}

export const demoCategories: Category[] = [
  { id: 1, name: "ChatGPT" },
  { id: 2, name: "Claude" },
  { id: 3, name: "Grok" },
  { id: 4, name: "更多好物" },
];

export const demoProducts: Product[] = [
  {
    id: 101,
    category_id: 1,
    name: "ChatGPT Plus 会员",
    subtitle: "让每一个好想法，都有回响",
    description:
      "解锁更强大的 AI 助手，陪你写作、学习与探索灵感。此商品为前端展示样例，具体服务内容以上线后的商品说明为准。",
    price_cents: 12900,
    original_price_cents: 15900,
    stock: 86,
    sales: 1280,
    kind: "ai",
    art: "spark",
    badge: "人气好物",
    tags: ["月度会员", "独享体验"],
  },
  ...[5, 20, 50].map(
    (multiplier, index): Product => ({
      id: 109 + index,
      category_id: 1,
      name: `ChatGPT Pro ${multiplier}x`,
      subtitle: `Pro 系列 · ${multiplier}x 套餐`,
      description: `ChatGPT Pro ${multiplier}x 套餐。具体服务内容、使用期限和价格以上架说明为准。`,
      price_cents: 0,
      original_price_cents: 0,
      stock: 0,
      sales: 0,
      kind: "ai",
      art: "spark",
      tags: ["Pro 系列", `${multiplier}x 套餐`],
    }),
  ),
  {
    id: 102,
    category_id: 2,
    name: "Claude Pro 会员",
    subtitle: "认真倾听，也懂你的奇思妙想",
    description:
      "长文阅读、创意写作与日常思考的灵感搭子。此商品为前端展示样例，具体权益与使用方式请以正式商品说明为准。",
    price_cents: 13900,
    original_price_cents: 16900,
    stock: 42,
    sales: 826,
    kind: "ai",
    art: "sun",
    badge: "小兔推荐",
    tags: ["月度会员", "创作搭子"],
  },
  {
    id: 112,
    category_id: 3,
    name: "Grok 会员",
    subtitle: "让好奇，发现更大的世界",
    description:
      "Grok 会员展示样例。请以下单时的商品说明、权益和兑换要求为准。",
    price_cents: 12800,
    original_price_cents: 0,
    stock: 20,
    sales: 0,
    kind: "ai",
    art: "spark",
    tags: ["月度会员", "数字商品"],
  },
  {
    id: 103,
    category_id: 4,
    name: "Gemini Advanced",
    subtitle: "和更大的世界，碰个面",
    description:
      "用 AI 探索更多可能，为学习与工作打开新的视角。此商品为前端展示样例，不代表任何平台的实际权益。",
    price_cents: 9900,
    original_price_cents: 12900,
    stock: 65,
    sales: 615,
    kind: "ai",
    art: "orbit",
    tags: ["月度会员", "多模态体验"],
  },
  {
    id: 104,
    category_id: 4,
    name: "YouTube Premium",
    subtitle: "把喜欢的时光，留给自己",
    description:
      "享受自在的影音时光，发现世界各地的精彩内容。此商品为前端展示样例，购买前请核对地区与使用要求。",
    price_cents: 2500,
    original_price_cents: 3500,
    stock: 128,
    sales: 2036,
    kind: "video",
    art: "play",
    badge: "好价精选",
    tags: ["月度会员", "畅享影音"],
  },
  {
    id: 105,
    category_id: 4,
    name: "Spotify Premium",
    subtitle: "给日常，添一首喜欢的歌",
    description:
      "让音乐陪你度过每个值得记录的小瞬间。此商品为前端展示样例，请以正式上架的服务范围为准。",
    price_cents: 1800,
    original_price_cents: 2800,
    stock: 96,
    sales: 1492,
    kind: "video",
    art: "music",
    tags: ["月度会员", "音乐相伴"],
  },
  {
    id: 106,
    category_id: 4,
    name: "Notion 效率工具包",
    subtitle: "把生活的碎片，整理成诗",
    description:
      "包含日程、阅读与习惯管理模板，让计划井井有条。此商品为前端展示样例，不提供真实下载内容。",
    price_cents: 1900,
    original_price_cents: 3900,
    stock: 200,
    sales: 368,
    kind: "tool",
    art: "book",
    badge: "新鲜上架",
    tags: ["数字模板", "即买即用"],
  },
  {
    id: 107,
    category_id: 4,
    name: "Canva 创意会员",
    subtitle: "你的灵感，值得被看见",
    description:
      "从日常海报到创意表达，轻松开启设计之旅。此商品为前端展示样例，服务权益以上线说明为准。",
    price_cents: 2900,
    original_price_cents: 4900,
    stock: 53,
    sales: 592,
    kind: "tool",
    art: "gem",
    tags: ["月度会员", "创意设计"],
  },
  {
    id: 108,
    category_id: 4,
    name: "游戏时光礼品卡",
    subtitle: "下一站，去快乐的世界",
    description:
      "送给自己的一份轻松时光。此商品为前端展示样例，不提供真实游戏卡密，请核对正式商品说明。",
    price_cents: 5000,
    original_price_cents: 5800,
    stock: 0,
    sales: 716,
    kind: "game",
    art: "game",
    tags: ["电子礼品卡", "快乐加倍"],
  },
];

export const faqs = [
  {
    q: "第一次来，怎么购买呀？",
    a: "在顶部切换商品类别并选择商品，查看商品说明，填写接收邮箱、确认数量，再核对并提交订单。正式开店后，支付成功的订单会自动发卡；当前样例商品仅用于体验前端流程，不会产生真实扣款。",
  },
  {
    q: "付款后，在哪里领取卡密？",
    a: "支付完成后，在「订单查询」中输入订单号与下单时填写的联系方式，即可查看订单状态。已完成的真实订单会展示卡密，请妥善保存，勿向他人透露。",
  },
  {
    q: "不小心关掉页面，订单还能找回来吗？",
    a: "可以。当前浏览器会保存最近的订单号，你可以到「订单查询」选中订单并填写原联系方式。如果更换了浏览器，需要手动输入订单号。请在下单后保存好订单号。",
  },
  {
    q: "购买前，有什么需要注意的？",
    a: "请仔细阅读商品的适用地区、兑换方式、使用期限和售后说明。数字商品在卡密发放后通常无法撤回，遇到问题可先查看订单状态和购买指南，再通过店铺公布的联系方式联系店主。",
  },
];

export const money = (cents: number) =>
  (cents / 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
