// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）；客户端给出的偏移与长度改用 bounds 包在 uint64 中判断边界。

//go:build linux || darwin

package parent

import (
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

// adsXattrPrefix namespaces NTFS alternate-data-stream (ADS) contents stored as
// Linux user xattrs. macOS/Windows clients address streams via the NTFS syntax
// `file:stream:$DATA`; we persist each stream's bytes under
// `user.gosamba.ads.<stream>` so they survive across handles (and reboots)
// instead of living in an ephemeral in-memory buffer.
const adsXattrPrefix = "user.gosamba.ads."

// eaXattrPrefix is the Linux namespace for ordinary extended attributes (EAs)
// set via SET_INFO FileFullEaInformation. NTFS EA names are case-insensitive
// ASCII; we lower-case and prefix with "user." so they live alongside other
// user xattrs but are still distinguishable from our ADS storage.
const eaXattrPrefix = "user."

// errXattrUnsupported is returned (wrapped) when the underlying filesystem does
// not support extended attributes (ENOTSUP). Callers and tests can use
// errors.Is to skip gracefully rather than fail.
var errXattrUnsupported = errors.New("filesystem does not support extended attributes")

// streamXattrName maps an NTFS stream name to its backing Linux xattr name.
func streamXattrName(stream string) string {
	return adsXattrPrefix + stream
}

// eaXattrName maps an NTFS EA name to its backing Linux xattr name. NTFS EA
// names are ASCII and case-insensitive; we sanitize to lower-case and strip any
// characters that are illegal in a Linux xattr name (NUL, '/').
func eaXattrName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if r == 0 || r == '/' {
			return -1
		}
		return r
	}, name)
	return eaXattrPrefix + name
}

// classifyXattrErr converts a raw unix errno from a get/set/list/remove call
// into one of: nil (no data / not present -> treat as empty), errXattrUnsupported,
// or the original error.
func classifyXattrErr(err error) error {
	if err == nil {
		return nil
	}
	// "Attribute does not exist" is ENODATA on Linux and ENOATTR on
	// macOS/BSD; isXattrNotFound is build-tagged per OS to test the right
	// errno (Linux's x/sys/unix does not export ENOATTR).
	if isXattrNotFound(err) {
		// Attribute not present — caller treats as empty.
		return nil
	}
	if errors.Is(err, unix.ENOTSUP) {
		return errXattrUnsupported
	}
	return err
}

// getxattr reads the full value of a single xattr. A missing attribute returns
// (nil, nil). ENOTSUP returns errXattrUnsupported.
func getxattr(path, name string) ([]byte, error) {
	// First size query, then read.
	size, err := unix.Getxattr(path, name, nil)
	if err != nil {
		if e := classifyXattrErr(err); e == nil {
			return nil, nil
		} else {
			return nil, e
		}
	}
	if size == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, size)
	n, err := unix.Getxattr(path, name, buf)
	if err != nil {
		if e := classifyXattrErr(err); e == nil {
			return nil, nil
		} else {
			return nil, e
		}
	}
	return buf[:n], nil
}

// getxattrSize returns the length in bytes of a single xattr's value without
// reading it. A missing attribute returns (0, nil). ENOTSUP returns
// errXattrUnsupported.
//
// This exists because callers that only want a size were paying for the value:
// getxattr issues a sizing Getxattr and then a second one that copies the whole
// attribute into memory. For an AAPL resource fork that is the entire fork read
// per directory entry, per QUERY_DIRECTORY, purely to call len() on it.
func getxattrSize(path, name string) (int, error) {
	size, err := unix.Getxattr(path, name, nil)
	if err != nil {
		if e := classifyXattrErr(err); e != nil {
			return 0, e
		}
		// Attribute not present — same as zero bytes for sizing purposes.
		return 0, nil
	}
	return size, nil
}

// streamXattrSize returns the byte length of the named stream's content on path
// without reading the content. A stream that was never written returns
// (0, nil) — same as readStreamXattr's (nil, nil).
func streamXattrSize(path, stream string) (int, error) {
	return getxattrSize(path, streamXattrName(stream))
}

// afpInfoStreamName is the NTFS stream name of Apple's metadata blob. macOS
// keeps a file's Finder Info there, and this server persists it like any other
// ADS stream (user.gosamba.ads.AFP_AfpInfo).
const afpInfoStreamName = "AFP_AfpInfo"

// finderInfoSize is the size of a Mac FinderInfo + ExtendedFinderInfo pair.
const finderInfoSize = 32

// afpInfoFinderOffset is where that 32-byte FinderInfo sits inside the 60-byte
// AFP_AfpInfo blob, after the signature, version, reserved and backup-time
// fields (Samba's MacExtensions.h struct AFPInfo).
const afpInfoFinderOffset = 16

// readAFPFinderInfo returns the FinderInfo stored in path's AFP_AfpInfo stream.
// ok is false when the stream was never written, the filesystem has no xattrs,
// or the stored blob is too short to hold a FinderInfo — all of which mean
// "this entry has no Finder Info", which callers report as zeros rather than
// as an error.
//
// This is one Getxattr into a stack buffer rather than the getxattr() helper's
// size-then-read pair: it runs once per entry of every AAPL directory listing,
// the blob is a fixed 60 bytes, and the second syscall would only fetch what
// the first already could have. A blob somehow longer than 60 bytes (ERANGE)
// falls back to the sizing read so an over-long stream still reports its
// Finder Info instead of silently reporting none.
func readAFPFinderInfo(path string) (fi [finderInfoSize]byte, ok bool) {
	var buf [afpInfoSize]byte
	n, err := unix.Getxattr(path, streamXattrName(afpInfoStreamName), buf[:])
	if err != nil {
		if !errors.Is(err, unix.ERANGE) {
			return fi, false
		}
		blob, gerr := getxattr(path, streamXattrName(afpInfoStreamName))
		if gerr != nil || len(blob) < afpInfoFinderOffset+finderInfoSize {
			return fi, false
		}
		copy(fi[:], blob[afpInfoFinderOffset:])
		return fi, true
	}
	if n < afpInfoFinderOffset+finderInfoSize {
		return fi, false
	}
	copy(fi[:], buf[afpInfoFinderOffset:])
	return fi, true
}

// listXattrNames enumerates all xattr names on path. A filesystem without xattr
// support returns errXattrUnsupported.
func listXattrNames(path string) ([]string, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		if e := classifyXattrErr(err); e == nil {
			return nil, nil
		} else {
			return nil, e
		}
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	n, err := unix.Listxattr(path, buf)
	if err != nil {
		if e := classifyXattrErr(err); e == nil {
			return nil, nil
		} else {
			return nil, e
		}
	}
	// Listxattr returns NUL-separated names.
	var names []string
	for _, raw := range strings.Split(string(buf[:n]), "\x00") {
		if raw != "" {
			names = append(names, raw)
		}
	}
	return names, nil
}

// readStreamXattr returns the persisted bytes for the named stream on path.
// A stream that was never written returns (nil, nil).
func readStreamXattr(path, stream string) ([]byte, error) {
	return getxattr(path, streamXattrName(stream))
}

// writeStreamXattr persists data as the named stream's content. A zero-length
// write still creates the attribute so the stream "exists".
//
// The write goes through a descriptor opened with O_NOFOLLOW where possible.
// The path-based unix.Setxattr follows symlinks, so a symlink swapped in after
// the CREATE-time ResolveSecure check would let a client's stream bytes land
// on a file outside the share. A leaf that has become a symlink (ELOOP) is
// refused outright; any other open failure — a file whose mode bits deny read
// but not write, say — falls back to the path form, which is no worse than the
// behaviour this replaced.
func writeStreamXattr(path, stream string, data []byte) error {
	f, oerr := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	switch {
	case oerr == nil:
		defer f.Close()
		return classifySetxattrErr(unix.Fsetxattr(int(f.Fd()), streamXattrName(stream), data, 0))
	case errors.Is(oerr, syscall.ELOOP):
		return oerr
	}
	return classifySetxattrErr(unix.Setxattr(path, streamXattrName(stream), data, 0))
}

// classifySetxattrErr normalizes a set-xattr failure: ENOTSUP (filesystem has
// no xattr support) becomes errXattrUnsupported so callers can skip silently;
// everything else is passed through so it can be reported to the client.
func classifySetxattrErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ENOTSUP) {
		return errXattrUnsupported
	}
	return err
}

// streamExists reports whether the named stream is currently stored on path.
//
// readStreamXattr cannot answer this: it maps "attribute not present" to
// (nil, nil), which is indistinguishable from a stream that exists but holds
// zero bytes — and CREATE has to tell those apart to honour FILE_OPEN vs
// FILE_CREATE. A filesystem without xattr support returns errXattrUnsupported;
// no stream can exist there.
func streamExists(path, stream string) (bool, error) {
	_, err := unix.Getxattr(path, streamXattrName(stream), nil)
	if err == nil {
		return true, nil
	}
	if isXattrNotFound(err) {
		return false, nil
	}
	if errors.Is(err, unix.ENOTSUP) {
		return false, errXattrUnsupported
	}
	return false, err
}

// removeStreamXattr deletes the named stream. A missing stream is not an error.
func removeStreamXattr(path, stream string) error {
	err := unix.Removexattr(path, streamXattrName(stream))
	if e := classifyXattrErr(err); e != nil {
		return e
	}
	return nil
}

// streamInfo is one enumerated alternate data stream.
type streamInfo struct {
	Name string
	Size int
}

// listStreams enumerates persisted ADS streams on path: every xattr under the
// adsXattrPrefix, with the prefix stripped to recover the NTFS stream name and
// its stored byte size.
func listStreams(path string) ([]streamInfo, error) {
	names, err := listXattrNames(path)
	if err != nil {
		return nil, err
	}
	var out []streamInfo
	for _, n := range names {
		if !strings.HasPrefix(n, adsXattrPrefix) {
			continue
		}
		stream := strings.TrimPrefix(n, adsXattrPrefix)
		if stream == "" {
			continue
		}
		size, gerr := unix.Getxattr(path, n, nil)
		if gerr != nil {
			if e := classifyXattrErr(gerr); e != nil {
				return nil, e
			}
			size = 0
		}
		out = append(out, streamInfo{Name: stream, Size: size})
	}
	return out, nil
}

// eaInfo is one extended attribute as exposed to SMB clients.
type eaInfo struct {
	Name  string
	Value []byte
}

// setEA persists an extended attribute under "user.<name>".
func setEA(path, name string, val []byte) error {
	return classifySetxattrErr(unix.Setxattr(path, eaXattrName(name), val, 0))
}

// setEAOnFile is setEA against an already-open descriptor. SET_INFO runs on a
// handle whose file CREATE opened with O_NOFOLLOW; writing the EA through that
// descriptor keeps the guarantee, whereas the path-based form re-resolves and
// would follow a symlink swapped in since. f is nil for directory handles,
// which have no descriptor.
func setEAOnFile(f *os.File, path, name string, val []byte) error {
	if f == nil {
		return setEA(path, name, val)
	}
	return classifySetxattrErr(unix.Fsetxattr(int(f.Fd()), eaXattrName(name), val, 0))
}

// listEAs enumerates user EAs on path, EXCLUDING our internal ADS storage
// (user.gosamba.ads.*) and any other gosamba-internal names. The returned EA
// name has the "user." prefix stripped.
func listEAs(path string) ([]eaInfo, error) {
	names, err := listXattrNames(path)
	if err != nil {
		return nil, err
	}
	var out []eaInfo
	for _, n := range names {
		if !strings.HasPrefix(n, eaXattrPrefix) {
			continue // skip non-user namespaces (system.*, security.*, trusted.*)
		}
		if strings.HasPrefix(n, adsXattrPrefix) {
			continue // skip our ADS-backing storage
		}
		val, gerr := getxattr(path, n)
		if gerr != nil {
			return nil, gerr
		}
		out = append(out, eaInfo{Name: strings.TrimPrefix(n, eaXattrPrefix), Value: val})
	}
	return out, nil
}

// parseFullEaList decodes a FILE_FULL_EA_INFORMATION chained list (MS-FSCC
// §2.4.15). Each entry: NextEntryOffset(4), Flags(1), EaNameLength(1),
// EaValueLength(2), EaName(ASCII, NUL-terminated), EaValue. A zero
// NextEntryOffset marks the last entry. Malformed buffers are truncated safely.
func parseFullEaList(buf []byte) []eaInfo {
	var out []eaInfo
	off := 0
	for off+8 <= len(buf) {
		next := binary.LittleEndian.Uint32(buf[off:])
		nameLen := int(buf[off+5])
		valLen := int(binary.LittleEndian.Uint16(buf[off+6:]))
		nameStart := off + 8
		valStart := nameStart + nameLen + 1 // skip NUL terminator
		if nameStart+nameLen > len(buf) || valStart+valLen > len(buf) {
			break
		}
		name := string(buf[nameStart : nameStart+nameLen])
		val := append([]byte{}, buf[valStart:valStart+valLen]...)
		if name != "" {
			out = append(out, eaInfo{Name: name, Value: val})
		}
		if next == 0 {
			break
		}
		// 下一项偏移来自客户端，在 uint64 中确认不越过缓冲区（32 位平台上 int 转换可能变成负数）
		nextOff, _, ok := bounds.Span(uint64(off)+uint64(next), 0, len(buf))
		if !ok {
			break
		}
		off = nextOff
	}
	return out
}

// encodeFullEaList encodes EAs into a FILE_FULL_EA_INFORMATION chained list
// (MS-FSCC §2.4.15). Returns a zero-length slice for no EAs (valid "no EAs"
// response). EA names are emitted upper-cased ASCII (NTFS convention) and
// 4-byte aligned per entry except the last.
func encodeFullEaList(eas []eaInfo) []byte {
	if len(eas) == 0 {
		return []byte{}
	}
	var out []byte
	for i, ea := range eas {
		name := []byte(strings.ToUpper(ea.Name))
		entry := make([]byte, 8+len(name)+1+len(ea.Value))
		// out[off+0..3] NextEntryOffset (filled below), out[off+4] Flags=0
		entry[5] = byte(len(name))
		binary.LittleEndian.PutUint16(entry[6:], uint16(len(ea.Value)))
		copy(entry[8:], name)
		// entry[8+len(name)] = NUL (already zero)
		copy(entry[8+len(name)+1:], ea.Value)
		if i < len(eas)-1 {
			for len(entry)%4 != 0 {
				entry = append(entry, 0)
			}
			binary.LittleEndian.PutUint32(entry[0:], uint32(len(entry)))
		}
		out = append(out, entry...)
	}
	return out
}

// encodeStreamInfoList builds a FILE_STREAM_INFORMATION list (MS-FSCC §2.4.40):
// the default ::$DATA entry (size = file size) followed by one :<name>:$DATA
// entry per persisted ADS stream. Each entry:
// NextEntryOffset(4), StreamNameLength(4), StreamSize(8),
// StreamAllocationSize(8), StreamName (UTF-16LE).
//
// A directory has no unnamed data stream, so the ::$DATA entry is omitted for
// one and only its named streams are reported — macOS keeps Finder metadata
// (tags, labels, icons) in streams on folders, and `xattr -l <dir>` enumerates
// them through this list. Returns an empty (not nil) buffer when there is
// nothing to report, which is a valid "no streams" answer.
func encodeStreamInfoList(path string, fileSize int64) []byte {
	type sentry struct {
		name string
		size int64
	}
	var entries []sentry
	if st, err := os.Lstat(path); err != nil || !st.IsDir() {
		entries = append(entries, sentry{name: "::$DATA", size: fileSize})
	}
	if streams, err := listStreams(path); err == nil {
		for _, s := range streams {
			entries = append(entries, sentry{name: ":" + s.Name + ":$DATA", size: int64(s.Size)})
		}
	}

	out := []byte{}
	for i, e := range entries {
		nameU16 := utf16leName(e.name)
		const fixed = 24
		rec := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint32(rec[4:], uint32(len(nameU16)))
		binary.LittleEndian.PutUint64(rec[8:], uint64(e.size))
		binary.LittleEndian.PutUint64(rec[16:], uint64(e.size))
		copy(rec[fixed:], nameU16)
		// Pad each record to an 8-byte boundary; set NextEntryOffset on all but
		// the last.
		for len(rec)%8 != 0 {
			rec = append(rec, 0)
		}
		if i < len(entries)-1 {
			binary.LittleEndian.PutUint32(rec[0:], uint32(len(rec)))
		}
		out = append(out, rec...)
	}
	return out
}
