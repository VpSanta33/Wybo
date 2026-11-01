import { useState, type ReactNode } from "react";
import { MoonRabbit } from "./Illustrations";
import { productBrand, productTier } from "./productBrands";
import type { Product } from "./data";

export function BrandImage({
  name,
  className = "",
}: {
  name: string;
  className?: string;
}) {
  const brand = productBrand(name);
  if (!brand) return null;
  return (
    <img
      className={`brand-image ${className}`}
      src={brand.image}
      alt=""
      aria-hidden="true"
      width="24"
      height="24"
    />
  );
}

export function ProductThumbnail({ product }: { product: Product }) {
  const [failed, setFailed] = useState(false);
  if (!product.cover && !productBrand(product.name)) return null;
  return (
    <span className="product-thumbnail">
      {product.cover && !failed ? (
        <img
          src={product.cover}
          alt=""
          aria-hidden="true"
          width="28"
          height="28"
          onError={() => setFailed(true)}
        />
      ) : (
        <BrandImage name={product.name} />
      )}
    </span>
  );
}

function CoverImage({
  src,
  name,
  fallback,
}: {
  src: string;
  name: string;
  fallback: ReactNode;
}) {
  const [failed, setFailed] = useState(false);
  return failed ? (
    fallback
  ) : (
    <div className="custom-product-cover">
      <img
        src={src}
        alt={`${name}商品图片`}
        onError={() => setFailed(true)}
        width="480"
        height="460"
      />
    </div>
  );
}

export default function ProductVisual({
  product,
  categoryName,
  variant,
}: {
  product?: Product;
  categoryName: string;
  variant: number;
}) {
  const brand = productBrand(product?.name ?? categoryName);
  const artwork = brand ? (
    <div className={`brand-visual visual-brand-${brand.id}`}>
      <span className="brand-visual-orbit" aria-hidden="true" />
      <span className="brand-visual-orbit orbit-inner" aria-hidden="true" />
      <div className="brand-visual-logo">
        <img
          src={brand.image}
          alt={`${brand.name}产品标志`}
          width="160"
          height="160"
        />
      </div>
      <span className="brand-visual-tier">
        {productTier(product?.name ?? categoryName)}
      </span>
      <div className="brand-visual-bunny" aria-hidden="true">
        <MoonRabbit variant={variant} />
      </div>
    </div>
  ) : (
    <MoonRabbit variant={variant} />
  );
  return product?.cover ? (
    <CoverImage
      key={product.cover}
      src={product.cover}
      name={product.name}
      fallback={artwork}
    />
  ) : (
    artwork
  );
}
