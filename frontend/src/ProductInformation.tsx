import { useLayoutEffect, useRef, useState } from "react";
import { BookOpen, ChevronRight } from "lucide-react";
import { Modal } from "./components";
import type { Product } from "./data";

function ReaderText({ text }: { text: string }) {
  // Captured separators are either blank lines or standalone 【headings】.
  const blocks = text.split(/(\n\s*\n|^[ \t]*【[^【】\n]+】[ \t]*$)/m);
  return blocks.map((block, index) => {
    const content = block.trim();
    if (!content) return null;
    if (index % 2 === 1) return <h3 key={index}>{content}</h3>;
    return <p key={index}>{content}</p>;
  });
}

export default function ProductInformation({ product }: { product: Product }) {
  const preview = useRef<HTMLDivElement>(null);
  const [clipped, setClipped] = useState(false);
  const [reading, setReading] = useState(false);
  const guide = product.redeem_guide?.trim();
  const title = guide ? "商品说明与兑换方式" : "商品说明";
  const text = [product.description?.trim(), guide && `【兑换方式】\n${guide}`]
    .filter(Boolean)
    .join("\n\n")
    .replace(/\r\n?/g, "\n");
  const excerpt = text.replace(/\n\s*\n/g, "\n");

  useLayoutEffect(() => {
    const frame = preview.current;
    const content = frame?.firstElementChild;
    if (!frame || !content) {
      setClipped(false);
      return;
    }
    const measure = () => setClipped(content.scrollHeight > frame.clientHeight + 1);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(content);
    observer.observe(frame);
    return () => observer.disconnect();
  }, [excerpt]);

  return (
    <section className="showcase-details" aria-label={title}>
      <h3>
        <BookOpen size={17} aria-hidden="true" />
        {title}
      </h3>
      {excerpt && (
        <div className="description-preview" ref={preview}>
          <p>{excerpt}</p>
        </div>
      )}
      {(product.tags.length > 0 || clipped) && (
        <div className="description-footer">
          {product.tags.length > 0 && (
            <div className="showcase-tags">
              {product.tags.map((tag) => <span key={tag}>{tag}</span>)}
            </div>
          )}
          {clipped && (
            <button
              type="button"
              className="description-open"
              aria-haspopup="dialog"
              onClick={() => setReading(true)}
            >
              查看完整说明 <ChevronRight size={15} aria-hidden="true" />
            </button>
          )}
        </div>
      )}
      {reading && (
        <Modal title="完整商品说明" onClose={() => setReading(false)}>
          <div className="description-reader">
            <header>
              <h2>{title}</h2>
              <p>{product.name}</p>
            </header>
            <div
              className="description-reader-body"
              role="region"
              aria-label="说明正文"
              tabIndex={0}
            >
              <ReaderText text={text} />
            </div>
          </div>
        </Modal>
      )}
    </section>
  );
}
