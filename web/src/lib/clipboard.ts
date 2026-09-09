// copyTextToClipboard 将文本写入浏览器剪贴板，并兼容不支持 Clipboard API 的浏览器。
// 输入：text 是待复制的文本。
// 输出：复制成功返回 true，否则返回 false。
// 副作用：请求浏览器剪贴板权限，必要时创建临时 textarea。
export async function copyTextToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 旧浏览器或权限受限时，退回临时 textarea 复制。
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus({ preventScroll: true });
  textarea.select();
  textarea.setSelectionRange(0, text.length);
  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}
