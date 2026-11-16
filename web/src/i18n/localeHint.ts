// 生成首屏语言包预加载脚本：页面在 <head> 里同步执行它，按已保存的语言偏好或浏览器语言，
// 尽早给对应语言包插入 modulepreload，使语言包与主包并行下载（否则要等主包执行后才发现）。
// 做成外部脚本而不是内联脚本，是为了不需要 Content-Security-Policy 的 'unsafe-inline' 或哈希例外。
// 匹配规则与 localeMeta.ts 的 pickLocale 保持一致，tests/localeHint.test.mjs 校验两者结果相同。
// 仅在构建期（vite.config.ts）与测试里使用，不进入前端包。

// files：语言代码 -> 语言包文件名（与脚本同目录，如 locale-en-abc123.js）；
// locales：本次构建包含的语言，顺序即匹配优先级；storageKey：保存界面语言的 localStorage 键
export function localeHintScript(
  files: Record<string, string>,
  locales: { code: string; match: string[] }[],
  fallback: string,
  storageKey: string,
): string {
  const table = locales.map((l) => [l.code, l.match])
  return `(function(){try{
var f=${JSON.stringify(files)},m=${JSON.stringify(table)},c=null,s=null,p,i,j,k,l,h;
var u=document.currentScript.src;
try{s=localStorage.getItem(${JSON.stringify(storageKey)})}catch(e){}
if(s&&f[s]){c=s}
if(!c){p=navigator.languages&&navigator.languages.length?navigator.languages:[navigator.language];
for(i=0;i<p.length&&!c;i++){l=String(p[i]).toLowerCase();
for(j=0;j<m.length&&!c;j++){for(k=0;k<m[j][1].length;k++){h=m[j][1][k];if(l===h||l.indexOf(h+"-")===0){c=m[j][0];break}}}}}
c=c||${JSON.stringify(fallback)};
if(!f[c]){return}
l=document.createElement("link");l.rel="modulepreload";l.crossOrigin="";l.href=u.slice(0,u.lastIndexOf("/")+1)+f[c];
document.head.appendChild(l)
}catch(e){}})();
`
}
