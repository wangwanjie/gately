package webpage

import (
	"fmt"
	"html/template"
	"sync"
)

// lazyTemplate 是首次使用时才解析的模板。错误页、登录页、导航面板与各内置风格的模板合计约 2 MB，
// 大多数实例只会用到其中一两个（不少实例一个都用不到），启动时全部解析只会白占内存。
// 模板源码都是包内常量，语法错误由 TestTemplatesParse 在测试中发现；运行时解析失败直接 panic，与原先在初始化时 panic 一致。
type lazyTemplate struct {
	name string
	// srcs 依次解析到同一个模板集中（外壳或共用片段在前，页面在后）
	srcs []string
	once sync.Once
	t    *template.Template
	err  error
}

func newLazyTemplate(name string, srcs ...string) *lazyTemplate {
	return &lazyTemplate{name: name, srcs: srcs}
}

// parse 首次调用时解析模板，返回模板与解析错误。
func (l *lazyTemplate) parse() (*template.Template, error) {
	l.once.Do(func() {
		t := template.New(l.name)
		for _, src := range l.srcs {
			if _, err := t.Parse(src); err != nil {
				l.err = fmt.Errorf("%s: %w", l.name, err)
				return
			}
		}
		l.t = t
	})
	return l.t, l.err
}

// get 返回解析好的模板，解析失败时 panic（模板源码是包内常量，不会因输入而失败）。
func (l *lazyTemplate) get() *template.Template {
	t, err := l.parse()
	if err != nil {
		panic(err)
	}
	return t
}

// CheckTemplates 解析全部已注册的页面、内置错误页风格与导航面板模板，返回遇到的第一个错误。
// 模板改为首次使用时解析后，语法错误不再在启动时暴露，由测试调用它提前发现。
func CheckTemplates() error {
	mu.RLock()
	all := make([]*lazyTemplate, 0, len(pages)+len(styleList)+1)
	for _, p := range pages {
		all = append(all, p)
	}
	mu.RUnlock()
	for _, s := range styleList {
		all = append(all, s.tpl)
	}
	all = append(all, panelTpl)
	for _, l := range all {
		if _, err := l.parse(); err != nil {
			return err
		}
	}
	return nil
}
