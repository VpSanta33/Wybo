import { useState, type ReactNode } from "react";
import { IslandVisual } from "./Illustrations";
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
  variant,
}: {
  product: Product;
  variant: number;
}) {
  const brand = productBrand(product.name);
  const artwork = brand ? (
    <div className={`brand-visual visual-brand-${brand.id}`}>
      <div className="brand-visual-logo">
        <img
          src={brand.image}
          alt={`${brand.name}产品标志`}
          width="160"
          height="160"
        />
      </div>
      <span className="brand-visual-tier">
        {productTier(product.name)}
      </span>
    </div>
  ) : (
    <IslandVisual variant={variant} />
  );
  return product.cover ? (
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
