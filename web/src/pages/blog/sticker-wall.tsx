import { useEffect, useRef, type CSSProperties } from "react";
import { setupStickerWall } from "./sticker-interaction";
import "./sticker-wall.css";

const stickers = [
  { id: "snowman", label: "雪人", src: "snowman.5367b9f4.webp", shadow: "snowman-shadow.0b515ece.webp", width: 52, height: 118 * 2 / 3, shadowScale: 2 / 3, top: "697px", left: "calc(50% + 195px)", rotation: "-4deg" },
  { id: "cat", label: "猫猫", src: "cat.fc8dd6bf.webp", shadow: "cat-shadow.ff377ea6.webp", width: 64, height: 65, top: "502px", left: "37.5%", rotation: "2deg" },
  { id: "long-cat", label: "长猫", src: "long-cat.png", shadow: "long-cat.png", width: 100, height: 166 * 2 / 3, top: "650px", left: "37.5%", rotation: "-3deg" },
  { id: "dog", label: "小狗", src: "dog.png", shadow: "dog.png", width: 110, height: 110 * 1130 / 1392, top: "379px", left: "37.5%", rotation: "5deg" },
  { id: "david-tao", label: "陶喆", src: "david-tao.png", shadow: "david-tao.png", width: 72, height: 72 * 1183 / 1329, top: "249px", left: "37.5%", rotation: "4deg" },
  { id: "robert-de-niro", label: "罗伯特德尼罗", src: "robert-de-niro.png", shadow: "robert-de-niro.png", width: 85, height: 85 * 683 / 778, top: "84px", left: "56.25%", rotation: "-4deg" },
];

export function StickerWall() {
  const wall = useRef<HTMLElement>(null);
  useEffect(() => { if (wall.current) return setupStickerWall(wall.current); }, []);
  return <aside ref={wall} className="sticker-region" aria-label="可拖拽贴纸" data-sticker-wall data-ready="false">
    <div className="sticker-stage" data-sticker-stage>{stickers.map((sticker, index) => <span className="sticker-anchor" data-sticker-anchor={sticker.id} key={sticker.id} style={{ "--desktop-left": sticker.left ?? "50%", "--desktop-top": sticker.top } as CSSProperties}>
      <button className="sticker-shell" type="button" aria-label={`拖动${sticker.label}贴纸；方向键微调，Shift 加速，Home 复位`} data-sticker={sticker.id} data-shadow-pad={32 * (sticker.shadowScale ?? 1)} data-dragging="false" data-settling="false" style={{ "--sticker-width": `${sticker.width}px`, "--sticker-rotation": sticker.rotation, "--enter-delay": `${80 + index * 55}ms`, "--shadow-pad": `${32 * (sticker.shadowScale ?? 1)}px`, "--shadow-scale": sticker.shadowScale ?? 1, "--sticker-mask": `url("/blog-static/stickers/${sticker.src}")` } as CSSProperties}>
        <span className="sticker-enter"><span className="sticker-lift"><span className="sticker-bob"><span className="sticker-bend">
          <img className={`sticker-shadow${sticker.shadow === sticker.src ? " sticker-shadow-generated" : ""}`} src={`/blog-static/stickers/${sticker.shadow}`} alt="" aria-hidden="true" decoding="async" data-sticker-image />
          <img className="sticker-image" src={`/blog-static/stickers/${sticker.src}`} alt={`${sticker.label}贴纸`} width={sticker.width} height={sticker.height} decoding="async" draggable="false" data-sticker-image />
          <span className="sticker-crease" aria-hidden="true" />
        </span></span></span></span>
      </button>
    </span>)}</div>
    <div className="sticker-controls" aria-label="贴纸控制">
      <button type="button" className="sticker-control" aria-label="恢复贴纸位置" data-sticker-reset><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7v5h5M5.6 16.5A8 8 0 1 0 6 6.8L4 9" /></svg></button>
      <button type="button" className="sticker-control sticker-sound" aria-label="开关贴纸音效" aria-pressed="true" data-sticker-sound data-enabled="true"><svg className="sound-on" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 10v4h4l5 4V6L9 10zM17 9a4 4 0 0 1 0 6M19 6a8 8 0 0 1 0 12" /></svg><svg className="sound-off" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 10v4h4l5 4V6L9 10zM17 9l4 4M21 9l-4 4" /></svg></button>
      <a className="sticker-control sticker-credit" href="https://buycoffee.top/" target="_blank" rel="noopener noreferrer" title="来自Hamster1963的品味" aria-label="来自Hamster1963的品味"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="18" cy="5" r="2" /><circle cx="6" cy="12" r="2" /><circle cx="18" cy="19" r="2" /><path d="m8 11 8-5M8 13l8 5" /></svg></a>
    </div>
  </aside>;
}
