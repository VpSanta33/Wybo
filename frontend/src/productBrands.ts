export interface ProductBrand {
  id: string;
  name: string;
  image: string;
}

const brands: ProductBrand[] = [
  { id: "chatgpt", name: "ChatGPT", image: "/brands/chatgpt.svg" },
  { id: "claude", name: "Claude", image: "/brands/claude.svg" },
  { id: "grok", name: "Grok", image: "/brands/grok.svg" },
  { id: "gemini", name: "Gemini", image: "/brands/gemini.svg" },
];

export function productBrand(name: string) {
  return brands.find((brand) => name.toLowerCase().includes(brand.id));
}

export function productTier(name: string) {
  const pro = name.match(/\bpro\s*(5|20|50)\s*x\b/i);
  if (pro) return `PRO ${pro[1]}x`;
  if (/\bplus\b/i.test(name)) return "PLUS";
  if (/\bpro\b/i.test(name)) return "PRO";
  return "MEMBERSHIP";
}

// A zero price means the merchant has not priced this product yet.
export const pricePending = (product: { price_cents: number }) =>
  product.price_cents <= 0;
