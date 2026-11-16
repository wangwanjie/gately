# 社交预览图

`social-preview.html` 是 GitHub 仓库社交预览图（Settings → General → Social preview）的源文件，尺寸 1280×640，右侧截图引用 `docs/images/en/overview.webp`（英文界面）。修改后在仓库根目录用 Chrome 无头模式重新生成：

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu --hide-scrollbars --allow-file-access-from-files --force-device-scale-factor=1 --window-size=1280,640 --screenshot=docs/images/social-preview.png "file://$PWD/docs/social/social-preview.html"
```
