import type { BlogImage } from "@/lib/blog";
export type StatusImageItem = { id: string; preview: string; file?: File; image?: BlogImage };

// 文件名和 MIME 可能不准确（例如微信 origin 图片），检查实际文件头。
export async function validateStatusFile(file: File) {
  if (file.size > 10 * 1024 * 1024) throw new Error(`${file.name}：单张图片最大 10 MiB。`);
  const bytes = new Uint8Array(await file.slice(0, 12).arrayBuffer());
  const text = String.fromCharCode(...bytes);
  const supported = (bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff)
    || (bytes[0] === 0x89 && text.slice(1, 8) === "PNG\r\n\x1a\n")
    || text.startsWith("GIF87a") || text.startsWith("GIF89a")
    || (text.startsWith("RIFF") && text.slice(8, 12) === "WEBP");
  if (!supported) throw new Error(`${file.name}：仅支持 JPEG、PNG、WebP 或 GIF 图片。`);
}

// 仅由发布/保存调用。逐张记住成功结果，后续失败重试不重复上传。
export async function prepareStatusImages(
  items: StatusImageItem[],
  upload: (file: File, progress: (percent: number) => void) => Promise<BlogImage>,
  uploaded: (id: string, image: BlogImage) => void,
  progress: (index: number, percent: number) => void,
) {
  const images: BlogImage[] = [];
  for (const [index, item] of items.entries()) {
    let image = item.image;
    if (!image) {
      if (!item.file) throw new Error("图片文件已失效，请重新选择。");
      image = await upload(item.file, (percent) => progress(index, percent));
      uploaded(item.id, image);
    }
    images.push(image);
  }
  return images;
}
