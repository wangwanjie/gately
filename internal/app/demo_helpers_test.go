package app

import "os"

func pidOf() int { return os.Getpid() }

func writeFileForTest(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }

func readFileForTest(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}
