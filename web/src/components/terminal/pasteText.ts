// 粘贴文本处理的纯函数，不依赖界面组件，便于单元测试。

// sanitizePaste 去掉粘贴内容中的 ESC 与除 Tab、回车、换行以外的 C0 控制字符以及 DEL。
// 括号粘贴模式下终端只在两端加 ESC[200~ / ESC[201~，正文里藏的 ESC[201~ 会提前结束括号粘贴，
// 让后面的行被 shell 直接执行，绕过多行确认；\x0f 等控制字符在单行里也能触发执行。
export function sanitizePaste(text: string): string {
  // eslint-disable-next-line no-control-regex
  return text.replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, '')
}
