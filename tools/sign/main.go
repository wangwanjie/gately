// sign 是发布签名工具：生成 minisign 格式的密钥对、给文件签名、验证签名，纯 Go 实现，CI 中不需要安装 minisign。
//
//	go run ./tools/sign keygen -dir ~/.gately-signing
//	go run ./tools/sign sign -t "gately v1.0.0" dist/checksums.txt      # 私钥与口令取自环境变量
//	go run ./tools/sign verify dist/checksums.txt                         # 用编进程序的受信公钥验证
//
// sign 的私钥取自环境变量 GATELY_SIGNING_KEY（minisign 私钥文件的完整内容）或 -key 指定的文件，
// 口令取自环境变量 GATELY_SIGNING_KEY_PASSWORD 或 -password-file 指定的文件。
package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/minisign"
	"github.com/wangwanjie/gately/internal/releasesig"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

// keygenKDF 为生成私钥时的口令派生参数，默认与 minisign -G 相同；测试中改小以免每次派生耗时数秒。
var keygenKDF = minisign.DefaultKDF

func usage() {
	fmt.Fprintln(os.Stderr, "用法: sign keygen -dir <目录> | sign sign [-key 文件] [-password-file 文件] [-t 受信说明] <文件> | sign verify [-pub 公钥] <文件>")
	os.Exit(2)
}

// keygen 在 dir 中生成 gately.key（口令加密的私钥）、gately.pub 与 password.txt（随机口令），目录 0700、文件 0600，已存在时拒绝覆盖。
func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	dir := fs.String("dir", "", "密钥目录")
	_ = fs.Parse(args)
	if *dir == "" {
		return errors.New("需要 -dir")
	}
	keyPath, pubPath, pwPath := filepath.Join(*dir, "gately.key"), filepath.Join(*dir, "gately.pub"), filepath.Join(*dir, "password.txt")
	for _, p := range []string{keyPath, pubPath, pwPath} {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s 已存在，不覆盖", p)
		}
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(*dir, 0o700); err != nil {
		return err
	}
	pw := make([]byte, 32)
	if _, err := rand.Read(pw); err != nil {
		return err
	}
	password := base64.RawURLEncoding.EncodeToString(pw)
	k, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	sk, err := minisign.EncodeSecretKey(k, password, keygenKDF, rand.Reader)
	if err != nil {
		return err
	}
	pub := k.Public()
	files := []struct {
		path string
		data []byte
	}{{pwPath, []byte(password + "\n")}, {keyPath, sk}, {pubPath, pub.File()}}
	for _, f := range files {
		// O_EXCL 防止并发生成时覆盖
		out, err := os.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = out.Write(f.data)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	fmt.Printf("密钥 ID: %s\n公钥: %s\n已写入 %s、%s、%s\n", pub.ID, pub, keyPath, pubPath, pwPath)
	return nil
}

// readPassword 从文件或环境变量读取口令，去掉末尾换行。
func readPassword(file string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return strings.TrimRight(os.Getenv("GATELY_SIGNING_KEY_PASSWORD"), "\r\n"), nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	keyFile := fs.String("key", "", "私钥文件，留空时读取环境变量 GATELY_SIGNING_KEY")
	pwFile := fs.String("password-file", "", "口令文件，留空时读取环境变量 GATELY_SIGNING_KEY_PASSWORD")
	trusted := fs.String("t", "", "受信说明，默认与 minisign 相同（时间戳与文件名）")
	out := fs.String("o", "", "签名文件路径，默认 <文件>.minisig")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("需要一个待签名的文件")
	}
	target := fs.Arg(0)
	var skData []byte
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			return err
		}
		skData = b
	} else {
		skData = []byte(os.Getenv("GATELY_SIGNING_KEY"))
	}
	if len(strings.TrimSpace(string(skData))) == 0 {
		return errors.New("没有私钥：请用 -key 指定或设置 GATELY_SIGNING_KEY")
	}
	password, err := readPassword(*pwFile)
	if err != nil {
		return err
	}
	k, err := minisign.ParseSecretKey(skData, password)
	if err != nil {
		return err
	}
	msg, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	tc := *trusted
	if tc == "" {
		tc = fmt.Sprintf("timestamp:%d\tfile:%s\thashed", time.Now().Unix(), filepath.Base(target))
	}
	sig, err := k.Sign(msg, "signature from gately release key "+k.ID.String(), tc)
	if err != nil {
		return err
	}
	if *out == "" {
		*out = target + ".minisig"
	}
	if err := os.WriteFile(*out, sig, 0o644); err != nil {
		return err
	}
	fmt.Printf("已用密钥 %s 签名 %s -> %s\n", k.ID, target, *out)
	return nil
}

// verify 验证 <文件>.minisig；不指定 -pub 时使用编进程序的受信公钥，规则与在线升级相同。
func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pub := fs.String("pub", "", "公钥文件或 base64 公钥，留空时使用编进程序的受信公钥")
	sigPath := fs.String("x", "", "签名文件路径，默认 <文件>.minisig")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("需要一个待验证的文件")
	}
	target := fs.Arg(0)
	if *sigPath == "" {
		*sigPath = target + ".minisig"
	}
	msg, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(*sigPath)
	if err != nil {
		return err
	}
	keys := releasesig.TrustedKeys()
	if *pub != "" {
		text := *pub
		if b, err := os.ReadFile(*pub); err == nil {
			text = string(b)
		}
		pk, err := minisign.ParsePublicKey(text)
		if err != nil {
			return err
		}
		keys = []string{pk.String()}
	}
	id, err := releasesig.Verify(keys, msg, sig)
	if err != nil {
		return err
	}
	fmt.Printf("签名有效（密钥 %s）\n", id)
	return nil
}
