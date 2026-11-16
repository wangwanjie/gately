package i18n

import (
	"io"
)

type lineWriter struct {
	w    io.Writer
	lang func() string
}

// NewWriter 返回按服务端语言翻译每次写入内容的 Writer，用于标准日志与命令行输出。
func NewWriter(w io.Writer) io.Writer {
	return &lineWriter{w: w, lang: ServerLang}
}

// NewWriterFor 与 NewWriter 类似，但使用固定语言。
func NewWriterFor(w io.Writer, lang string) io.Writer {
	return &lineWriter{w: w, lang: func() string { return lang }}
}

func (l *lineWriter) Write(p []byte) (int, error) {
	lang := l.lang()
	if lang == SourceLang || !hasHan(p) {
		return l.w.Write(p)
	}
	// 逐行翻译，行尾换行原样保留，见 TrLines
	if _, err := io.WriteString(l.w, TrLines(lang, string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}
